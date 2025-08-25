// 文件路径: couponkill-coupon-service/src/main/java/com/aliyun/seckill/couponkillcouponservice/service/CouponService.java
package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.pojo.Coupon;
import java.util.List;

public interface CouponService {
    List<Coupon> getAvailableCoupons();
    List<Coupon> list();
    Coupon createCoupon(Coupon coupon);
    Coupon getCouponById(Long id);
    boolean grantCoupons(List<Long> userIds);

    // 新增方法
    boolean deductStock(Long couponId);
    boolean increaseStock(Long couponId);
    void updateStock(Long couponId, int newStock);
    void handleExpiredCoupons();

    // TCC相关方法
    boolean lockStock(Long couponId);
    boolean confirmDeductStock(Long couponId);
    boolean releaseStock(Long couponId);

    /**
     * 扣减库存并返回使用的虚拟分片ID
     * @param couponId 优惠券ID
     * @return 使用的虚拟分片ID
     */
    String deductStockWithVirtualId(Long couponId);



}
