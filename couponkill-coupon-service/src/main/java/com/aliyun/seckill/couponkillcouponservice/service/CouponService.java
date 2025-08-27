package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.pojo.Coupon;

import java.util.List;

public interface CouponService {
    /**
     * 获取所有可用优惠券
     */
    List<Coupon> getAvailableCoupons();

    /**
     * 获取所有优惠券
     */
    List<Coupon> list();

    /**
     * 创建优惠券（会自动创建多个分片）
     */
    Coupon createCoupon(Coupon coupon);

    /**
     * 根据ID获取优惠券（合并所有分片信息）
     */
    Coupon getCouponById(Long id);

    /**
     * 批量发放优惠券
     */
    boolean grantCoupons(List<Long> userIds);

    /**
     * 扣减库存（从多个分片中选择一个）
     */
    boolean deductStock(Long couponId);

    /**
     * 增加库存
     */
    boolean increaseStock(Long couponId);

    /**
     * 更新库存
     */
    void updateStock(Long couponId, int newStock);

    /**
     * 处理过期优惠券
     */
    void handleExpiredCoupons();

    /**
     * 扣减库存并返回分片索引
     */
    Integer deductStockWithShardIndex(Long couponId);
    
    /**
     * 扣减库存并返回分片ID（couponId_shardIndex格式）
     */
    String deductStockWithShardId(Long couponId);
    
    /**
     * 获取优惠券的所有分片
     */
    List<Coupon> getCouponShards(Long id);
}
