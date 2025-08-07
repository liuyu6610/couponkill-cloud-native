// com.aliyun.seckill.coupon.controller.CouponController.java
package com.aliyun.seckill.coupon.controller;

import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.coupon.service.CouponService;
import com.aliyun.seckill.pojo.Coupon;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/coupon")
@Tag(name = "优惠券管理", description = "优惠券查询、库存操作接口")
public class CouponController {

    @Autowired
    private CouponService couponService;

    @GetMapping("/available")
    @Operation(summary = "获取所有可用优惠券")
    public Result<List<Coupon>> getAvailableCoupons() {
        return Result.success(couponService.getAvailableCoupons());
    }

    @GetMapping("/{id}")
    @Operation(summary = "根据ID获取优惠券")
    public Result<Coupon> getCouponById(@PathVariable Long id) {
        return Result.success(couponService.getCouponById(id));
    }

    @PostMapping("/deduct/{id}")
    @Operation(summary = "扣减优惠券库存")
    public Result<Boolean> deductStock(@PathVariable Long id) {
        return Result.success(couponService.deductStock(id));
    }

    @PostMapping("/increase/{id}")
    @Operation(summary = "增加优惠券库存")
    public Result<Boolean> increaseStock(@PathVariable Long id) {
        return Result.success(couponService.increaseStock(id));
    }
}