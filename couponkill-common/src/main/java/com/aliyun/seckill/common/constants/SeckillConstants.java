package com.aliyun.seckill.common.constants;

/**
 * 秒杀系统常量类
 */
public class SeckillConstants {

    // Redis 键前缀
    public static final String REDIS_PRODUCT_PREFIX = "seckill:product:";
    public static final String REDIS_STOCK_PREFIX = "seckill:stock:";
    public static final String REDIS_SECKILL_STOCK_PREFIX = "seckill:seckill:stock:";
    public static final String REDIS_USER_SECKILL_PREFIX = "seckill:user:seckill:";
    public static final String REDIS_SECKILL_URL_PREFIX = "seckill:url:";
    
    // 布隆过滤器名称
    public static final String BLOOM_FILTER_PRODUCT = "seckill:product:bloom";
    
    // JWT相关
    public static final String JWT_TOKEN_PREFIX = "Bearer ";
    public static final long JWT_EXPIRE_TIME = 24 * 60 * 60 * 1000; // 24小时
    
    // 商品类型
    public static final int PRODUCT_TYPE_NORMAL = 0; // 常驻商品
    public static final int PRODUCT_TYPE_SECKILL = 1; // 秒杀商品
    
    // 订单状态
    public static final int ORDER_STATUS_PENDING = 0; // 待支付
    public static final int ORDER_STATUS_PAID = 1; // 已支付
    public static final int ORDER_STATUS_CANCELLED = 2; // 已取消
    public static final int ORDER_STATUS_REFUNDED = 3; // 已退款
    
    // 秒杀活动状态
    public static final int SECKILL_STATUS_NOT_STARTED = 0; // 未开始
    public static final int SECKILL_STATUS_IN_PROGRESS = 1; // 进行中
    public static final int SECKILL_STATUS_FINISHED = 2; // 已结束
    public static final int SECKILL_STATUS_CANCELLED = 3; // 已取消
    
    // RocketMQ 主题
    public static final String ROCKETMQ_ORDER_TOPIC = "seckill.order.topic";
    public static final String ROCKETMQ_STOCK_TOPIC = "seckill_stock_topic";
    
    // 分布式锁前缀
    public static final String LOCK_PRODUCT_PREFIX = "seckill:lock:product:";
    
    // 服务名称
    public static final String SERVICE_USER = "user-service";
    public static final String SERVICE_PRODUCT = "product-service";
    public static final String SERVICE_ORDER = "order-service";
    public static final String SERVICE_ADMIN = "admin-service";
    public static final String SERVICE_GATEWAY = "seckill-gateway";
}
    