package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface CouponMapper {
    List<Coupon> selectAvailableCoupons();
    Coupon selectCouponById(@Param("id") Long id);
    int insertCoupon(Coupon coupon);
    int batchGrantCoupons(@Param("userIds") List<Long> userIds);
}
