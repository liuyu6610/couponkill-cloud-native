package com.aliyun.seckill.couponkillcouponservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@Tag(name = "优惠券管理", description = "优惠券相关操作接口")
@RestController
@RequestMapping("/api/v1/coupon")
public class CouponController {
    private final CouponService couponService;

    public CouponController(CouponService couponService) {
        this.couponService = couponService;
    }

    @Operation(summary = "查询可用优惠券列表", description = "获取所有可用的优惠券")
    @GetMapping("/available")
    public ApiResponse<List<Coupon>> getAvailableCoupons() {
        return ApiResponse.success(couponService.getAvailableCoupons());
    }

    @Operation(summary = "创建优惠券", description = "创建一个新的优惠券")
    @PostMapping("/create")
    public ApiResponse<Coupon> createCoupon(
            @Parameter(description = "优惠券信息") @RequestBody Coupon coupon) {
        return ApiResponse.success(couponService.createCoupon(coupon));
    }

    @Operation(summary = "根据ID查询优惠券", description = "根据优惠券ID获取优惠券详情")
    @GetMapping("/{id}")
    public ApiResponse<Coupon> getCouponById(
            @Parameter(description = "优惠券ID") @PathVariable Long id) {
        return ApiResponse.success(couponService.getCouponById(id));
    }

    @Operation(summary = "后台管理接口：批量发放优惠券", description = "向指定用户列表批量发放优惠券")
    @PostMapping("/admin/grant")
    public ApiResponse<Boolean> grantCoupons(
            @Parameter(description = "用户ID列表") @RequestBody List<Long> userIds) {
        return ApiResponse.success(couponService.grantCoupons(userIds));
    }
}
