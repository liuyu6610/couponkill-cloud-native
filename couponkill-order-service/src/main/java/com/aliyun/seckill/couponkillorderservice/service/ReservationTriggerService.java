package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.couponkillorderservice.domain.ReservationStatuses;
import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.SeckillReservationMapper;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.LocalDateTime;
import java.util.List;
import java.util.concurrent.ThreadLocalRandom;

/**
 * 到点帮抢：认领 PENDING → 调 enterSeckillAsync → 回写 QUEUED/FAILED。
 * 禁止改 Lua；多实例靠 DB 乐观锁 + Redis 锁。
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class ReservationTriggerService {

    private final SeckillReservationMapper reservationMapper;
    private final OrderService orderService;
    private final CouponServiceFeignClient couponServiceFeignClient;
    private final AsyncSeckillEnterService asyncSeckillEnterService;
    private final StringRedisTemplate stringRedisTemplate;
    private final NotificationService notificationService;

    @Value("${couponkill.reservation.max-retry:3}")
    private int maxRetry;

    @Value("${couponkill.reservation.batch-size:50}")
    private int batchSize;

    public void fireDueReservations() {
        LocalDateTime now = LocalDateTime.now();
        List<SeckillReservation> due = reservationMapper.selectDuePending(now, batchSize);
        if (due.isEmpty()) {
            return;
        }
        log.info("预约帮抢扫描到 {} 条待触发", due.size());
        for (SeckillReservation r : due) {
            // 轻微 jitter，防惊群
            try {
                Thread.sleep(ThreadLocalRandom.current().nextInt(0, 50));
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
            fireOne(r);
        }
    }

    public void syncQueuedResults() {
        List<SeckillReservation> queued = reservationMapper.selectQueuedForResultSync(batchSize);
        for (SeckillReservation r : queued) {
            if (r.getRequestId() == null || r.getRequestId().isBlank()) {
                continue;
            }
            String result = asyncSeckillEnterService.getResult(r.getRequestId());
            if (result == null || result.isBlank() || "PENDING".equals(result) || "UNKNOWN".equals(result)) {
                continue;
            }
            if (result.startsWith("SUCCESS")) {
                String orderId = result.contains(":") ? result.substring(result.indexOf(':') + 1) : null;
                reservationMapper.markSuccess(r.getId(), orderId, r.getVersion());
                notificationService.notifyReservation(
                        r.getUserId(), r.getId(), "RESERVATION_SUCCESS",
                        "预约帮抢成功",
                        "券 " + r.getCouponId() + " 已入队成功" + (orderId != null ? "，订单 " + orderId : ""));
                log.info("预约结果同步 SUCCESS: id={}, requestId={}, orderId={}",
                        r.getId(), r.getRequestId(), orderId);
            } else if (result.startsWith("FAIL")) {
                reservationMapper.markFailed(r.getId(), ErrorCodes.SYS_ERROR, "秒杀履约失败", r.getVersion());
                notificationService.notifyReservation(
                        r.getUserId(), r.getId(), "RESERVATION_FAILED",
                        "预约帮抢失败",
                        "券 " + r.getCouponId() + " 履约失败，请查看「我的预约」");
                log.info("预约结果同步 FAIL: id={}, requestId={}", r.getId(), r.getRequestId());
            }
        }
    }

    public void expireStale() {
        LocalDateTime now = LocalDateTime.now();
        List<SeckillReservation> stale = reservationMapper.selectExpiredPending(now, batchSize);
        for (SeckillReservation r : stale) {
            Coupon coupon = safeLoadCoupon(r.getCouponId());
            if (coupon != null && coupon.getSeckillEndAt() != null && now.isAfter(coupon.getSeckillEndAt())) {
                reservationMapper.markExpired(r.getId());
                notificationService.notifyReservation(
                        r.getUserId(), r.getId(), "RESERVATION_EXPIRED",
                        "预约已过期",
                        "券 " + r.getCouponId() + " 活动已结束，预约失效");
                log.info("预约过期: id={}, couponId={}", r.getId(), r.getCouponId());
            }
        }
    }

    /** 回收卡住的 FIRING（崩溃/超时），避免预约永久卡在抢购中 */
    public void reclaimStuckFiring() {
        LocalDateTime before = LocalDateTime.now().minusMinutes(2);
        int n = reservationMapper.reclaimStuckFiring(before, batchSize);
        if (n > 0) {
            log.warn("回收卡住 FIRING 预约 {} 条", n);
        }
    }

    private void fireOne(SeckillReservation r) {
        String lockKey = "reservation:fire:" + r.getId();
        Boolean locked = stringRedisTemplate.opsForValue()
                .setIfAbsent(lockKey, "1", Duration.ofSeconds(30));
        if (!Boolean.TRUE.equals(locked)) {
            return;
        }
        try {
            // 活动已结束 → EXPIRED
            Coupon coupon = safeLoadCoupon(r.getCouponId());
            LocalDateTime now = LocalDateTime.now();
            if (coupon != null && coupon.getSeckillEndAt() != null && !now.isBefore(coupon.getSeckillEndAt())) {
                reservationMapper.markExpired(r.getId());
                notificationService.notifyReservation(
                        r.getUserId(), r.getId(), "RESERVATION_EXPIRED",
                        "预约已过期",
                        "券 " + r.getCouponId() + " 活动已结束，预约失效");
                return;
            }

            int claimed = reservationMapper.claimForFire(r.getId(), r.getVersion());
            if (claimed == 0) {
                return;
            }
            // claim 后 version+1
            int currentVersion = r.getVersion() + 1;

            EnterSeckillResp resp;
            try {
                resp = orderService.enterSeckillAsync(r.getUserId(), r.getCouponId());
            } catch (Exception e) {
                log.warn("预约帮抢调用热路径异常: id={}, err={}", r.getId(), e.getMessage());
                handleTransientOrFail(r, currentVersion,
                        ErrorCodes.SYS_ERROR, "系统繁忙，请稍后由调度重试");
                return;
            }

            if (resp != null && "QUEUED".equals(resp.getStatus())) {
                reservationMapper.markQueued(r.getId(), resp.getRequestId(), currentVersion);
                log.info("预约帮抢入队成功: id={}, userId={}, couponId={}, requestId={}",
                        r.getId(), r.getUserId(), r.getCouponId(), resp.getRequestId());
                return;
            }

            int err = resp != null && resp.getErr() != null ? resp.getErr() : ErrorCodes.SYS_ERROR;
            String msg = resp != null && resp.getMessage() != null ? resp.getMessage() : "秒杀失败";

            // 冷却 / 未预热：有限重试
            if (err == ErrorCodes.COOLING_DOWN || err == ErrorCodes.NOT_PREHEATED) {
                handleTransientOrFail(r, currentVersion, err, msg);
                return;
            }

            reservationMapper.markFailed(r.getId(), err, msg, currentVersion);
            notificationService.notifyReservation(
                    r.getUserId(), r.getId(), "RESERVATION_FAILED",
                    "预约帮抢失败",
                    "券 " + r.getCouponId() + "：" + msg);
            log.info("预约帮抢失败: id={}, err={}, msg={}", r.getId(), err, msg);
        } finally {
            try {
                stringRedisTemplate.delete(lockKey);
            } catch (Exception ignore) {
                // ignore
            }
        }
    }

    private void handleTransientOrFail(SeckillReservation r, int version, int err, String msg) {
        int retries = r.getRetryCount() == null ? 0 : r.getRetryCount();
        if (retries + 1 >= maxRetry) {
            String finalMsg = msg + "（已达重试上限）";
            reservationMapper.markFailed(r.getId(), err, finalMsg, version);
            notificationService.notifyReservation(
                    r.getUserId(), r.getId(), "RESERVATION_FAILED",
                    "预约帮抢失败",
                    "券 " + r.getCouponId() + "：" + finalMsg);
            return;
        }
        reservationMapper.bumpRetry(r.getId(), version);
        reservationMapper.revertToPending(r.getId(), version + 1);
        log.info("预约帮抢瞬态失败，回 PENDING 待重试: id={}, retry={}, err={}", r.getId(), retries + 1, err);
    }

    private Coupon safeLoadCoupon(Long couponId) {
        try {
            ApiResponse<Coupon> resp = couponServiceFeignClient.getCouponById(couponId);
            return resp != null ? resp.getData() : null;
        } catch (Exception e) {
            log.warn("帮抢加载券失败: couponId={}", couponId, e.getMessage());
            return null;
        }
    }
}
