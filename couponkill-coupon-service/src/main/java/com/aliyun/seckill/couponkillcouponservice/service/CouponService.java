package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.pojo.Coupon;

import java.util.List;

public interface CouponService {
    List<Coupon> getAvailableCoupons();
    Coupon createCoupon(Coupon coupon);
    Coupon getCouponById(Long id);
    boolean grantCoupons(List<Long> userIds);
}
