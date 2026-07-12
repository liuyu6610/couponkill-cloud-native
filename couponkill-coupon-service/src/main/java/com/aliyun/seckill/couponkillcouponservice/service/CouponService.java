package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.connector.SyncStockResult;
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
     * 按 virtualId（couponId_shardIndex）回补秒杀分片库存
     */
    boolean increaseSeckillStockByShardId(String virtualId);

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
     * 扣减库存并返回分片ID（couponId_shardIndex格式），同时异步更新 Redis
     */
    String deductStockWithShardId(Long couponId);
    
    /**
     * 仅扣减 DB 分片秒杀库存并返回 virtualId（couponId_shardIndex）。
     * 不触碰 Redis coupon:stock（供热路径 Lua 已预扣后的异步消费者使用）。
     */
    String deductDbSeckillStockOnly(Long couponId);
    
    /**
     * 获取优惠券的所有分片
     */
    List<Coupon> getCouponShards(Long id);
    
    /**
     * 随机获取一个有库存的优惠券分片
     */
    Coupon getRandomAvailableShard(Long couponId);
    
    /**
     * 异步预热全部有效券库存到 Redis（coupon:stock 为全分片合计）
     */
    void asyncPreheatStockToRedis();

    /**
     * 预热单张券库存到 Redis（供秒杀热路径缺 key 时补救）
     */
    boolean preheatCouponStock(Long couponId);

    /**
     * 将目标库存写入 Redis coupon:stock:{couponId}（电商 Connector 旁路同步用）。
     * @param force true 强制覆盖；false 安全合并（永不抬高已有库存）
     * @return 含 appliedStock 的同步结果（失败时 success=false）
     */
    SyncStockResult syncRedisStock(Long couponId, Long targetStock, boolean force);
}