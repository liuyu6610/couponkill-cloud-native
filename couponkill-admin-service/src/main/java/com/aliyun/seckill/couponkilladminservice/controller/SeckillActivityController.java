// com.aliyun.seckill.admin.controller.SeckillActivityController.java
package com.aliyun.seckill.couponkilladminservice.controller;

import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.SeckillActivity;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkilladminservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkilladminservice.seckill.SeckillActivityService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/admin/seckill")
@Tag(name = "秒杀活动管理", description = "管理员操作秒杀活动接口")
public class SeckillActivityController {

    @Autowired
    private SeckillActivityService activityService;

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;

    @PostMapping("/activity")
    @Operation(summary = "创建秒杀活动")
    public Result<SeckillActivity> createActivity(@RequestBody SeckillActivity activity) {
        return Result.success(activityService.createActivity(activity));
    }

    @PutMapping("/activity/{id}/status")
    @Operation(summary = "更新秒杀活动状态")
    public Result<Boolean> updateActivityStatus(
            @PathVariable Long id,
            @RequestParam Integer status) {
        return Result.success(activityService.updateActivityStatus(id, status));
    }

    @GetMapping("/activities")
    @Operation(summary = "获取所有秒杀活动")
    public Result<List<SeckillActivity>> getAllActivities() {
        return Result.success(activityService.getAllActivities());
    }

    @PostMapping("/coupon/{id}/stock")
    @Operation(summary = "更新优惠券库存")
    public Result<?> updateCouponStock(
            @PathVariable Long id,
            @RequestParam int newStock) {
        couponServiceFeignClient.updateStock(id, newStock);
        return Result.success();
    }

    @GetMapping("/coupons")
    @Operation(summary = "获取所有优惠券")
    public Result<List<Coupon>> getAllCoupons() {
        return Result.success(couponServiceFeignClient.list());
    }
}