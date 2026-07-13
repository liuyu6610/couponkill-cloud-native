package com.aliyun.seckill.couponkillorderservice.job;

import com.aliyun.seckill.couponkillorderservice.service.ReservationTriggerService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/**
 * 预约帮抢定时任务：扫描 PENDING → 代发热路径；同步 QUEUED 结果；过期清理。
 * 多实例防重：单条认领靠 DB 乐观锁 + reservation:fire:{id} Redis 锁。
 */
@Slf4j
@Component
@RequiredArgsConstructor
@ConditionalOnProperty(name = "couponkill.reservation.enabled", havingValue = "true", matchIfMissing = true)
public class ReservationFireJob {

    private final ReservationTriggerService triggerService;

    @Scheduled(fixedDelayString = "${couponkill.reservation.scan-interval-ms:1000}")
    public void scanAndFire() {
        try {
            triggerService.fireDueReservations();
            triggerService.syncQueuedResults();
        } catch (Exception e) {
            log.warn("预约帮抢调度异常: {}", e.getMessage(), e);
        }
    }

    @Scheduled(fixedDelayString = "${couponkill.reservation.expire-interval-ms:60000}")
    public void expireStale() {
        try {
            triggerService.expireStale();
            triggerService.reclaimStuckFiring();
        } catch (Exception e) {
            log.warn("预约过期清理异常: {}", e.getMessage());
        }
    }
}
