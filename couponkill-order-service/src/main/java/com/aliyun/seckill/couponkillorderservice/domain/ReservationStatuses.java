package com.aliyun.seckill.couponkillorderservice.domain;

/**
 * 预约帮抢状态常量（与 PRODUCT-POSITIONING P0 对齐）。
 */
public final class ReservationStatuses {
    public static final String PENDING = "PENDING";
    public static final String FIRING = "FIRING";
    public static final String QUEUED = "QUEUED";
    public static final String SUCCESS = "SUCCESS";
    public static final String FAILED = "FAILED";
    public static final String CANCELLED = "CANCELLED";
    public static final String EXPIRED = "EXPIRED";

    private ReservationStatuses() {}
}
