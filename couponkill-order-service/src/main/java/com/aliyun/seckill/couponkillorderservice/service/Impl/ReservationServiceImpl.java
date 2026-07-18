package com.aliyun.seckill.couponkillorderservice.service.Impl;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillorderservice.domain.ReservationStatuses;
import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.SeckillReservationMapper;
import com.aliyun.seckill.couponkillorderservice.service.ReservationService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class ReservationServiceImpl implements ReservationService {

    private final SeckillReservationMapper reservationMapper;
    private final CouponServiceFeignClient couponServiceFeignClient;

    @Override
    @Transactional
    public SeckillReservation create(Long userId, Long couponId) {
        if (userId == null || couponId == null) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "userId/couponId 不能为空");
        }

        SeckillReservation existing = reservationMapper.selectActiveByUserAndCoupon(userId, couponId);
        if (existing != null) {
            return existing;
        }

        Coupon coupon = loadCoupon(couponId);
        if (coupon.getType() == null || coupon.getType() != 2) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "仅秒抢券支持预约帮抢");
        }
        if (coupon.getStatus() == null || coupon.getStatus() != 1) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "优惠券已失效");
        }
        if (coupon.getSeckillStartAt() == null || coupon.getSeckillEndAt() == null) {
            throw new BusinessException(ErrorCodes.INVALID_REQ,
                    "该秒杀券尚未配置活动时间窗，暂不可预约");
        }
        LocalDateTime now = LocalDateTime.now();
        if (!now.isBefore(coupon.getSeckillStartAt())) {
            throw new BusinessException(ErrorCodes.INVALID_REQ,
                    "活动已开售，请直接秒杀，不可再预约");
        }
        if (!now.isBefore(coupon.getSeckillEndAt())) {
            throw new BusinessException(ErrorCodes.ACTIVITY_ENDED, "活动已结束");
        }

        SeckillReservation r = new SeckillReservation();
        r.setUserId(userId);
        r.setCouponId(couponId);
        r.setStatus(ReservationStatuses.PENDING);
        r.setReserveAt(now);
        r.setTriggerAt(coupon.getSeckillStartAt());
        r.setRetryCount(0);
        r.setVersion(0);
        r.setCreateTime(now);
        r.setUpdateTime(now);

        try {
            reservationMapper.insert(r);
        } catch (DuplicateKeyException e) {
            SeckillReservation raced = reservationMapper.selectActiveByUserAndCoupon(userId, couponId);
            if (raced != null) {
                return raced;
            }
            throw new BusinessException(ErrorCodes.INVALID_REQ, "已存在活跃预约");
        }
        log.info("创建预约帮抢: id={}, userId={}, couponId={}, triggerAt={}",
                r.getId(), userId, couponId, r.getTriggerAt());
        return reservationMapper.selectById(r.getId());
    }

    @Override
    @Transactional
    public boolean cancel(Long userId, Long reservationId) {
        SeckillReservation r = reservationMapper.selectById(reservationId);
        if (r == null || !userId.equals(r.getUserId())) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "预约不存在");
        }
        if (!ReservationStatuses.PENDING.equals(r.getStatus())) {
            throw new BusinessException(ErrorCodes.INVALID_REQ,
                    "仅待开抢(PENDING)状态可取消，当前=" + r.getStatus());
        }
        int n = reservationMapper.markCancelled(reservationId, userId);
        return n > 0;
    }

    @Override
    public List<SeckillReservation> listMine(Long userId) {
        return reservationMapper.selectByUserId(userId);
    }

    @Override
    public SeckillReservation getById(Long userId, Long reservationId) {
        SeckillReservation r = reservationMapper.selectById(reservationId);
        if (r == null || !userId.equals(r.getUserId())) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "预约不存在");
        }
        return r;
    }

    @Override
    public void onFulfillResult(Long userId, Long couponId, String orderId, boolean success) {
        if (userId == null || couponId == null) {
            return;
        }
        if (success) {
            int n = reservationMapper.markSuccessByUserCoupon(userId, couponId, orderId);
            if (n > 0) {
                log.info("预约履约成功回写: userId={}, couponId={}, orderId={}", userId, couponId, orderId);
            }
        } else {
            int n = reservationMapper.markFailedByUserCoupon(userId, couponId, "履约失败");
            if (n > 0) {
                log.info("预约履约失败回写: userId={}, couponId={}", userId, couponId);
            }
        }
    }

    private Coupon loadCoupon(Long couponId) {
        try {
            ApiResponse<Coupon> resp = couponServiceFeignClient.getCouponById(couponId);
            if (resp == null || resp.getData() == null) {
                throw new BusinessException(ErrorCodes.COUPON_NOT_FOUND, "优惠券不存在");
            }
            return resp.getData();
        } catch (BusinessException e) {
            throw e;
        } catch (Exception e) {
            log.warn("查询优惠券失败: couponId={}", couponId, e);
            throw new BusinessException(ErrorCodes.COUPON_SERVICE_UNAVAILABLE, "优惠券服务不可用");
        }
    }
}
