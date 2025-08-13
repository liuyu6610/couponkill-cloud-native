package com.aliyun.seckill.couponkillcouponservice.controller;
import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import io.swagger.annotations.Api;
import io.swagger.annotations.ApiOperation;
import org.springframework.web.bind.annotation.*;

import java.util.List;

;

@Api(tags = "优惠券管理")
@RestController
@RequestMapping("/api/v1/coupon")
public class CouponController {

    private final CouponService couponService;

    public CouponController(CouponService couponService) {
        this.couponService = couponService;
    }

    @ApiOperation("查询可用优惠券列表")
    @GetMapping("/available")
    public ApiResponse<List<Coupon>> getAvailableCoupons() {
        return ApiResponse.success(couponService.getAvailableCoupons());
    }

    @ApiOperation("创建优惠券")
    @PostMapping("/create")
    public ApiResponse<Coupon> createCoupon(@RequestBody Coupon coupon) {
        return ApiResponse.success(couponService.createCoupon(coupon));
    }

    @ApiOperation("根据ID查询优惠券")
    @GetMapping("/{id}")
    public ApiResponse<Coupon> getCouponById(@PathVariable Long id) {
        return ApiResponse.success(couponService.getCouponById(id));
    }

    @ApiOperation("后台管理接口：批量发放优惠券")
    @PostMapping("/admin/grant")
    public ApiResponse<Boolean> grantCoupons(@RequestBody List<Long> userIds) {
        return ApiResponse.success(couponService.grantCoupons(userIds));
    }
}
