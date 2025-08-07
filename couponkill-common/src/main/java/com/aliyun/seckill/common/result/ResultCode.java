// com.aliyun.seckill.common.result.ResultCode.java
package com.aliyun.seckill.common.result;

import lombok.Getter;

@Getter
public enum ResultCode {
    SUCCESS(200, "操作成功"),
    FAIL(500, "操作失败"),
    PARAM_ERROR(400, "参数错误"),
    UNAUTHORIZED(401, "未授权"),
    FORBIDDEN(403, "禁止访问"),
    NOT_FOUND(404, "资源不存在"),
    SYSTEM_ERROR(500, "系统异常"),
    USER_EXIST(1001, "用户已存在"),
    USER_NOT_FOUND(1002, "用户不存在"),
    PASSWORD_ERROR(1003, "密码错误"),
    COUPON_NOT_FOUND(2001, "优惠券不存在"),
    COUPON_OUT_OF_STOCK(2002, "优惠券已抢完"),
    COUPON_LIMIT_EXCEEDED(2003, "优惠券数量已达上限"),
    SECKILL_NOT_START(3001, "秒杀未开始"),
    SECKILL_ENDED(3002, "秒杀已结束"),
    REPEAT_SECKILL(3003, "不可重复秒杀"),
    ORDER_NOT_FOUND(4001, "订单不存在");

    private final int code;
    private final String message;

    ResultCode(int code, String message) {
        this.code = code;
        this.message = message;
    }
}