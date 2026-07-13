package com.aliyun.seckill.common.api;
public interface ErrorCodes {
    public static final int COOLING_DOWN = 10001;   // 冷却期内
    public static final int OUT_OF_STOCK = 10002;   // 库存不足
    public static final int RATE_LIMITED = 10003;   // 频控
    public static final int INVALID_REQ   = 10004;  // 参数/状态非法
    public static final int NOT_PREHEATED = 10005;  // Redis 库存未预热
    public static final int NOT_STARTED   = 10006;  // 活动未开售
    public static final int ACTIVITY_ENDED = 10007; // 活动已结束
    public static final int AUTH_FAIL     = 20001;  // 鉴权失败
    public static final int SYS_ERROR     = 50000;  // 系统异常
}
