package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface CouponMapper {
    List<Coupon> selectAvailableCoupons();

    List<Coupon> selectAll();

    Coupon selectById(Long id);

    int insertCoupon(Coupon coupon);

    /**
     * @deprecated 无 shard_index 时 ShardingSphere 路由不稳定，请用 {@link #selectByCouponIdAndShardIndex}
     *             或 Service 层 {@code loadAllShards}。
     */
    @Deprecated
    List<Coupon> selectByCouponId(@Param("couponId") Long couponId);

    /** 按 id + shard_index 精确路由单分片（库存汇总真源） */
    Coupon selectByCouponIdAndShardIndex(@Param("couponId") Long couponId,
                                         @Param("shardIndex") Integer shardIndex);

    // 查询所有主分片用于缓存预热
    List<Coupon> selectMainShardsForCache();

    // 根据分片索引更新库存
    int updateStockByShardIndex(@Param("couponId") Long couponId, 
                               @Param("shardIndex") Integer shardIndex, 
                               @Param("change") int change, 
                               @Param("version") int version);

    /** 按分片回补秒杀库存（不校验 version，避免缓存版本陈旧导致回补失败） */
    int increaseSeckillStockByShardIndex(@Param("couponId") Long couponId,
                                         @Param("shardIndex") Integer shardIndex,
                                         @Param("change") int change);

    int updateStock(@Param("couponId") Long couponId, @Param("change") int change, @Param("version") int version);

    // 查询过期优惠券
    List<Coupon> selectExpiredCoupons(@Param("expireTime") LocalDateTime expireTime);

    /**
     * @deprecated 请用 {@link #updateCouponStatusByShardIndex}
     */
    @Deprecated
    int updateCouponStatus(@Param("couponId") Long couponId, @Param("status") int status);

    int updateCouponStatusByShardIndex(@Param("couponId") Long couponId,
                                       @Param("shardIndex") Integer shardIndex,
                                       @Param("status") int status);

    /**
     * @deprecated 请用 {@link #updateSeckillWindowByShardIndex}
     */
    @Deprecated
    int updateSeckillWindow(@Param("couponId") Long couponId,
                            @Param("seckillStartAt") LocalDateTime seckillStartAt,
                            @Param("seckillEndAt") LocalDateTime seckillEndAt);

    int updateSeckillWindowByShardIndex(@Param("couponId") Long couponId,
                                        @Param("shardIndex") Integer shardIndex,
                                        @Param("seckillStartAt") LocalDateTime seckillStartAt,
                                        @Param("seckillEndAt") LocalDateTime seckillEndAt);

    /**
     * @deprecated 请用 {@link #deleteCouponByIdAndShardIndex}
     */
    @Deprecated
    int deleteCouponById(@Param("id") Long id);

    int deleteCouponByIdAndShardIndex(@Param("id") Long id, @Param("shardIndex") Integer shardIndex);
}
