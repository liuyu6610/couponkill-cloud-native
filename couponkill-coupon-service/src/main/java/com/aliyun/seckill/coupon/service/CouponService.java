// com.aliyun.seckill.coupon.service.CouponService.java
package com.aliyun.seckill.coupon.service;

import com.aliyun.seckill.pojo.Coupon;
import com.baomidou.mybatisplus.extension.service.IService;

import java.util.List;

public interface CouponService extends IService<Coupon> {
    /**
     * 获取所有可用优惠券
     */
    List<Coupon> getAvailableCoupons();

    /**
     * 根据ID获取优惠券
     */
    Coupon getCouponById(Long couponId);

    /**
     * 扣减优惠券库存
     */
    boolean deductStock(Long couponId);

    /**
     * 增加优惠券库存
     */
    boolean increaseStock(Long couponId);

    /**
     * 更新优惠券库存(管理员操作)
     */
    void updateStock(Long couponId, int newStock);
}