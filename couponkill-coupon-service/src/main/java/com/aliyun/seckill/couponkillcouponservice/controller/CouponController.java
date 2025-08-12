// com.aliyun.seckill.coupon.controller.CouponController.java
package com.aliyun.seckill.couponkillcouponservice.controller;

import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
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

    // 添加更新库存的接口
    @PostMapping("/{id}/stock")
    @Operation(summary = "更新优惠券库存")
    public Result<Void> updateStock(@PathVariable Long id, @RequestParam int newStock) {
        couponService.updateStock(id, newStock);
        return Result.success();
    }

    @GetMapping
    @Operation(summary = "获取所有优惠券")
    public Result<List<Coupon>> list() {
        return Result.success(couponService.list());
    }
}
