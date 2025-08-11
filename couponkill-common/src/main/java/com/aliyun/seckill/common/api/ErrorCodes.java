package com.aliyun.seckill.common.api;
public interface ErrorCodes {
    int COOLING_DOWN = 10001;   // 冷却期内
    int OUT_OF_STOCK = 10002;   // 库存不足
    int RATE_LIMITED = 10003;   // 频控
    int INVALID_REQ   = 10004;  // 参数/状态非法
    int AUTH_FAIL     = 20001;  // 鉴权失败
    int SYS_ERROR     = 50000;  // 系统异常
}
