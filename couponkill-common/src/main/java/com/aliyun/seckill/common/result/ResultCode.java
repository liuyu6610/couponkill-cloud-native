// com.aliyun.seckill.common.result.ResultCode.java
package com.aliyun.seckill.common.result;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public enum ResultCode {
    SUCCESS(200, "成功"),
    FAIL(500, "失败"),
    PARAM_ERROR(400, "参数错误"),
    COUPON_NOT_FOUND(2001, "优惠券不存在"),
    COUPON_OUT_OF_STOCK(2002, "优惠券已抢完"),
    COUPON_LIMIT_EXCEEDED(2003, "优惠券数量已达上限"),
    REPEAT_SECKILL(3003, "不可重复秒杀"),
    ORDER_NOT_FOUND(4001, "订单不存在"),
    USER_NOT_FOUND(5001, "用户不存在"),
    USER_EXIST(5002, "用户已存在"),
    PASSWORD_ERROR(5003, "密码错误"),
    TOKEN_INVALID(6001, "令牌无效"),
    ACTIVITY_NOT_START(7001, "活动未开始"),
    ACTIVITY_ENDED(7002, "活动已结束"),
    SYSTEM_BUSY( 7003, "系统繁忙，请稍后再试"  );

    private final int code;
    private final String message;
}