package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
public class CouponServiceImpl implements CouponService {

    private final CouponMapper couponMapper;

    public CouponServiceImpl(CouponMapper couponMapper) {
        this.couponMapper = couponMapper;
    }

    @Override
    public List<Coupon> getAvailableCoupons() {
        return couponMapper.selectAvailableCoupons();
    }

    @Override
    public Coupon createCoupon(Coupon coupon) {
        couponMapper.insertCoupon(coupon);
        return coupon;
    }

    @Override
    public Coupon getCouponById(Long id) {
        return couponMapper.selectCouponById(id);
    }

    @Override
    public boolean grantCoupons(List<Long> userIds) {
        return couponMapper.batchGrantCoupons(userIds) > 0;
    }
}
