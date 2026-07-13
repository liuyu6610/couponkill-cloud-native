package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface SeckillReservationMapper {

    int insert(SeckillReservation reservation);

    SeckillReservation selectById(@Param("id") Long id);

    List<SeckillReservation> selectByUserId(@Param("userId") Long userId);

    SeckillReservation selectActiveByUserAndCoupon(@Param("userId") Long userId,
                                                   @Param("couponId") Long couponId);

    /** 扫描待触发：PENDING 且 trigger_at <= now */
    List<SeckillReservation> selectDuePending(@Param("now") LocalDateTime now,
                                              @Param("limit") int limit);

    /** 乐观锁认领：PENDING → FIRING */
    int claimForFire(@Param("id") Long id, @Param("version") Integer version);

    int markQueued(@Param("id") Long id,
                   @Param("requestId") String requestId,
                   @Param("version") Integer version);

    int markFailed(@Param("id") Long id,
                   @Param("failCode") Integer failCode,
                   @Param("failReason") String failReason,
                   @Param("version") Integer version);

    int markSuccess(@Param("id") Long id,
                    @Param("orderId") String orderId,
                    @Param("version") Integer version);

    int markCancelled(@Param("id") Long id, @Param("userId") Long userId);

    int markExpired(@Param("id") Long id);

    int bumpRetry(@Param("id") Long id, @Param("version") Integer version);

    /** 回滚 FIRING → PENDING（瞬态失败可重试） */
    int revertToPending(@Param("id") Long id, @Param("version") Integer version);

    List<SeckillReservation> selectQueuedForResultSync(@Param("limit") int limit);

    int markSuccessByUserCoupon(@Param("userId") Long userId,
                                @Param("couponId") Long couponId,
                                @Param("orderId") String orderId);

    int markFailedByUserCoupon(@Param("userId") Long userId,
                               @Param("couponId") Long couponId,
                               @Param("failReason") String failReason);

    /** 过期仍 PENDING 的预约 */
    List<SeckillReservation> selectExpiredPending(@Param("now") LocalDateTime now,
                                                  @Param("limit") int limit);

    /** 卡住的 FIRING（进程崩溃/超时），回 PENDING 重试 */
    int reclaimStuckFiring(@Param("before") LocalDateTime before, @Param("limit") int limit);
}
