package com.aliyun.seckill.couponkillcouponservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.math.BigDecimal;
import java.time.LocalDateTime;
import java.util.List;
@Slf4j
@Tag(name = "优惠券管理", description = "优惠券相关操作接口")
@RestController
@RequestMapping("/api/v1/coupon")
public class CouponController {
    @Autowired
    private CouponService couponService;
    @Operation(summary = "查询可用优惠券列表", description = "获取所有可用的优惠券")
    @GetMapping("/available")
    public ApiResponse<List<Coupon>> getAvailableCoupons() {
        return ApiResponse.success(couponService.getAvailableCoupons());
    }

    @Operation(summary = "创建优惠券", description = "创建一个新的优惠券")
@PostMapping("/create")
public ApiResponse<Coupon> createCoupon(
        @Parameter(description = "优惠券名称") @RequestParam String name,
        @Parameter(description = "优惠券描述") @RequestParam(required = false) String description,
        @Parameter(description = "类型(1-常驻,2-秒抢)") @RequestParam Integer type,
        @Parameter(description = "面值(元)") @RequestParam BigDecimal faceValue,
        @Parameter(description = "最低消费(元)") @RequestParam BigDecimal minSpend,
        @Parameter(description = "有效期(天)") @RequestParam Integer validDays,
        @Parameter(description = "每人限领数量") @RequestParam Integer perUserLimit,
        @Parameter(description = "总库存") @RequestParam Integer totalStock,
        @Parameter(description = "秒杀总库存(仅秒抢类型有效)") @RequestParam(required = false) Integer seckillTotalStock) {
    Coupon coupon = new Coupon();
    coupon.setName(name);
    coupon.setDescription(description);
    coupon.setType(type);
    coupon.setFaceValue(faceValue);
    coupon.setMinSpend(minSpend);
    coupon.setValidDays(validDays);
    coupon.setPerUserLimit(perUserLimit);
    coupon.setTotalStock(totalStock);
    coupon.setSeckillTotalStock(seckillTotalStock);
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
    @Operation(summary = "锁定优惠券库存")
    @PostMapping("/lock/{id}")
    public ApiResponse<Boolean> lockStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        // 实现库存锁定逻辑
        // 这里可以使用Redis分布式锁或其他机制来实现
        try {
            boolean success = couponService.deductStock(id); // 或者使用其他锁定机制
            return ApiResponse.success(success);
        } catch (Exception e) {
            log.error("锁定库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "锁定库存失败");
        }
    }

    @Operation(summary = "确认扣减优惠券库存")
    @PostMapping("/confirm/{id}")
    public ApiResponse<Boolean> confirmDeductStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        // 实现确认扣减库存逻辑
        try {
            // TCC Confirm阶段，通常为空实现或执行确认操作
            // 这里可以添加一些确认逻辑，如记录日志等
            log.info("确认扣减优惠券库存，couponId: {}", id);
            return ApiResponse.success(true);
        } catch (Exception e) {
            log.error("确认扣减库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "确认扣减库存失败: " + e.getMessage());
        }
    }


    @Operation(summary = "释放优惠券库存")
    @PostMapping("/release/{id}")
    public ApiResponse<Boolean> releaseStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        // 实现释放库存逻辑
        // 这里通常是TCC事务的Cancel阶段，需要释放之前锁定的库存
        try {
            boolean success = couponService.increaseStock(id);
            return ApiResponse.success(success);
        } catch (Exception e) {
            log.error("释放库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "释放库存失败");
        }
    }
    @Operation(summary = "扣减优惠券库存")
    @PostMapping("/deduct/{id}")
    public ApiResponse<Boolean> deductStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        try {
            boolean result = couponService.deductStock(id);
            return ApiResponse.success(result);
        } catch (Exception e) {
            log.error("扣减库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "扣减库存失败: " + e.getMessage());
        }
    }

    @Operation(summary = "增加优惠券库存")
    @PostMapping("/increase/{id}")
    public ApiResponse<Boolean> increaseStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        try {
            boolean result = couponService.increaseStock(id);
            return ApiResponse.success(result);
        } catch (Exception e) {
            log.error("增加库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "增加库存失败: " + e.getMessage());
        }
    }
    @Operation(summary = "扣减优惠券库存并返回使用的虚拟分片ID")
    @PostMapping("/deduct-with-virtual-id/{id}")
    public ApiResponse<String> deductStockWithVirtualId(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        try {
            String virtualId = couponService.deductStockWithVirtualId(id);
            if (virtualId != null) {
                return ApiResponse.success(virtualId);
            } else {
                return ApiResponse.fail(500, "扣减库存失败");
            }
        } catch (Exception e) {
            log.error("扣减库存并获取虚拟分片ID失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "扣减库存并获取虚拟分片ID失败: " + e.getMessage());
        }
    }

    @Operation(summary = "创建补偿优惠券")
    @PostMapping("/compensation")
    public ApiResponse<Void> createCompensationCoupon(@RequestBody Coupon compensationCoupon) {
        // 实现创建补偿优惠券的逻辑
        try {
            if (compensationCoupon == null) {
                return ApiResponse.fail(400, "请求参数不能为空");
            }

            // 设置默认值
            compensationCoupon.setType(1); // 常驻优惠券
            compensationCoupon.setStatus(1); // 有效状态
            compensationCoupon.setCreateTime(LocalDateTime.now());
            compensationCoupon.setUpdateTime(LocalDateTime.now());
            compensationCoupon.setVersion(0);

            // 如果没有设置有效期，默认设置为1天
            if (compensationCoupon.getValidDays() == null) {
                compensationCoupon.setValidDays(1);
            }

            // 如果没有设置面值，默认设置为10元
            if (compensationCoupon.getFaceValue() == null) {
                compensationCoupon.setFaceValue(BigDecimal.TEN);
            }

            // 如果没有设置最低消费，默认设置为0元
            if (compensationCoupon.getMinSpend() == null) {
                compensationCoupon.setMinSpend(BigDecimal.ZERO);
            }

            Coupon result = couponService.createCoupon(compensationCoupon);
            if (result != null) {
                log.info("创建补偿优惠券成功，couponId: {}", result.getId());
                return ApiResponse.success(null);
            } else {
                return ApiResponse.fail(500, "创建补偿优惠券失败");
            }
        } catch (Exception e) {
            log.error("创建补偿优惠券失败", e);
            return ApiResponse.fail(500, "创建补偿优惠券失败: " + e.getMessage());
        }
    }


}
