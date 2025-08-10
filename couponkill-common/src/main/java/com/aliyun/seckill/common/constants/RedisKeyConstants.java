package com.aliyun.seckill.common.constants;

public class RedisKeyConstants {
    // 优惠券库存键：coupon:stock:{couponId}
    public static final String COUPON_STOCK_KEY = "coupon:stock:";
    // 用户领取记录键：user:received:{userId}:{couponId}
    public static final String USER_RECEIVED_KEY = "user:received:";
    // 秒杀队列键：seckill:queue:{couponId}
    public static final String SECKILL_QUEUE_KEY = "seckill:queue:";
}
