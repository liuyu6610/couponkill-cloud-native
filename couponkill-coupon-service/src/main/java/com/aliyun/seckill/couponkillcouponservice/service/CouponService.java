package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.pojo.Coupon;
import java.util.List;
public interface CouponService {

        /**
         * 获取所有可用优惠券
         */
        List<Coupon> getAvailableCoupons ( );

        /**
         * 根据ID获取优惠券
         */
        Coupon getCouponById ( Long couponId );

        /**
         * 扣减优惠券库存
         */
        boolean deductStock ( Long couponId );

        /**
         * 增加优惠券库存
         */
        boolean increaseStock ( Long couponId );

        /**
         * 更新优惠券库存(管理员操作)
         */
        void updateStock ( Long couponId, int newStock );

        /**
         * 定时任务处理已过期优惠券
         */
        void handleExpiredCoupons ( );

         List<Coupon> list ();

}


