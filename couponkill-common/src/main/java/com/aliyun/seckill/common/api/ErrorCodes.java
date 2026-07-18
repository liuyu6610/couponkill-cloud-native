package com.aliyun.seckill.common.api;

/**
 * 对外业务错误码唯一真源（Phase3）。
 * <p>
 * 约定：成功用 {@link ApiResponse} 的 {@code code=0}；失败用本接口常量。
 * 历史 {@code ResultCode}（200/9001/2002…）仅保留兼容，新代码禁止再引入。
 */
public interface ErrorCodes {
    // —— 秒杀 / 库存 / 订单（10xxx）——
    int COOLING_DOWN = 10001;
    int OUT_OF_STOCK = 10002;
    int RATE_LIMITED = 10003;
    int INVALID_REQ = 10004;
    int NOT_PREHEATED = 10005;
    int NOT_STARTED = 10006;
    int ACTIVITY_ENDED = 10007;
    int COUPON_NOT_FOUND = 10008;
    int COUPON_LIMIT_EXCEEDED = 10009;
    int REPEAT_SECKILL = 10010;
    int ORDER_NOT_FOUND = 10011;
    int SECKILL_USE_DEDICATED_API = 10012;
    int COUPON_SERVICE_UNAVAILABLE = 10013;

    // —— 用户 / 鉴权（20xxx）——
    int AUTH_FAIL = 20001;
    int TOKEN_INVALID = 20002;
    int USER_NOT_FOUND = 20003;
    int USER_EXIST = 20004;
    int PASSWORD_ERROR = 20005;

    // —— 系统（50xxx）——
    int SYS_ERROR = 50000;
    int SYSTEM_BUSY = 50003;
}
