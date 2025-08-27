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
import java.util.concurrent.CompletableFuture;

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
        log.info("查询可用优惠券列表");
        return ApiResponse.success(couponService.getAvailableCoupons());
    }

    @Operation(summary = "创建优惠券", description = "创建一个新的优惠券")
    @PostMapping("/create")
    public ApiResponse<Coupon> createCoupon(
            @Parameter(description = "优惠券ID（可选，不传则自动生成）") @RequestParam(required = false) Long id,
            @Parameter(description = "优惠券名称") @RequestParam String name,
            @Parameter(description = "优惠券描述") @RequestParam(required = false) String description,
            @Parameter(description = "类型(1-常驻,2-秒抢)") @RequestParam Integer type,
            @Parameter(description = "面值(元)") @RequestParam BigDecimal faceValue,
            @Parameter(description = "最低消费(元)") @RequestParam BigDecimal minSpend,
            @Parameter(description = "有效期(天)") @RequestParam Integer validDays,
            @Parameter(description = "每人限领数量") @RequestParam Integer perUserLimit,
            @Parameter(description = "总库存") @RequestParam Integer totalStock,
            @Parameter(description = "秒杀总库存(仅秒抢类型有效)") @RequestParam(required = false) Integer seckillTotalStock) {

        log.info("创建优惠券: id={}, name={}, type={}, faceValue={}", id, name, type, faceValue);

        // 如果没有提供ID，则生成一个
        if (id == null) {
            id = Coupon.generateId();
        }
        
        Coupon coupon = new Coupon();
        coupon.setId(id);
        coupon.setName(name);
        coupon.setDescription(description);
        coupon.setType(type);
        coupon.setFaceValue(faceValue);
        coupon.setMinSpend(minSpend);
        coupon.setValidDays(validDays);
        coupon.setPerUserLimit(perUserLimit);
        coupon.setTotalStock(totalStock);
        coupon.setSeckillTotalStock(seckillTotalStock != null ? seckillTotalStock : 0);
        coupon.setRemainingStock(totalStock);
        coupon.setSeckillRemainingStock(seckillTotalStock != null ? seckillTotalStock : 0);

        // 确保必填字段有默认值
        coupon.setStatus(1); // 默认有效状态

        Coupon createdCoupon = couponService.createCoupon(coupon);
        return ApiResponse.success(createdCoupon);
    }

    @Operation(summary = "根据ID查询优惠券", description = "根据优惠券ID获取优惠券详情")
    @GetMapping("/{id}")
    public ApiResponse<Coupon> getCouponById(
            @Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("根据ID查询优惠券: id={}", id);
        return ApiResponse.success(couponService.getCouponById(id));
    }

    @Operation(summary = "获取优惠券的所有分片", description = "获取指定优惠券的所有分片信息")
    @GetMapping("/{id}/shards")
    public ApiResponse<List<Coupon>> getCouponShards(
            @Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("获取优惠券的所有分片: id={}", id);
        return ApiResponse.success(couponService.getCouponShards(id));
    }

    @Operation(summary = "随机获取一个有库存的优惠券分片", description = "随机获取指定优惠券中一个有库存的分片")
    @GetMapping("/{id}/random-shard")
    public ApiResponse<Coupon> getRandomAvailableShard(
            @Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("随机获取一个有库存的优惠券分片: id={}", id);
        Coupon shard = couponService.getRandomAvailableShard(id);
        if (shard != null) {
            return ApiResponse.success(shard);
        } else {
            return ApiResponse.fail(404, "未找到有库存的分片");
        }
    }

    @Operation(summary = "查询所有优惠券", description = "获取所有优惠券")
    @GetMapping("/list")
    public ApiResponse<List<Coupon>> list() {
        log.info("查询所有优惠券");
        return ApiResponse.success(couponService.list());
    }

    @Operation(summary = "后台管理接口：批量发放优惠券", description = "向指定用户列表批量发放优惠券")
    @PostMapping("/admin/grant")
    public ApiResponse<Boolean> grantCoupons(
            @Parameter(description = "用户ID列表") @RequestBody List<Long> userIds) {
        log.info("批量发放优惠券: userIds={}", userIds);
        return ApiResponse.success(couponService.grantCoupons(userIds));
    }

    @Operation(summary = "扣减优惠券库存")
    @PostMapping("/deduct/{id}")
    public ApiResponse<Boolean> deductStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("扣减优惠券库存: id={}", id);
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
        log.info("增加优惠券库存: id={}", id);
        try {
            boolean result = couponService.increaseStock(id);
            return ApiResponse.success(result);
        } catch (Exception e) {
            log.error("增加库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "增加库存失败: " + e.getMessage());
        }
    }

    @Operation(summary = "扣减优惠券库存并返回使用的分片ID")
    @PostMapping("/deduct-with-shard-id/{id}")
    public ApiResponse<String> deductStockWithShardId(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("扣减优惠券库存并返回使用的分片ID: id={}", id);
        try {
            String shardId = couponService.deductStockWithShardId(id);
            if (shardId != null) {
                return ApiResponse.success(shardId);
            } else {
                return ApiResponse.fail(500, "扣减库存失败");
            }
        } catch (Exception e) {
            log.error("扣减库存并获取分片ID失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "扣减库存并获取分片ID失败: " + e.getMessage());
        }
    }
    
    @Operation(summary = "异步扣减优惠券库存并返回使用的分片ID")
    @PostMapping("/async-deduct-with-shard-id/{id}")
    public ApiResponse<String> asyncDeductStockWithShardId(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("异步扣减优惠券库存并返回使用的分片ID: id={}", id);
        
        // 异步处理，立即返回结果
        CompletableFuture.supplyAsync(() -> {
            try {
                return couponService.deductStockWithShardId(id);
            } catch (Exception e) {
                log.error("异步扣减库存并获取分片ID失败，couponId: {}", id, e);
                return null;
            }
        }).thenAccept(result -> {
            if (result == null) {
                log.warn("异步扣减库存失败，未获取到分片ID: couponId={}", id);
            } else {
                log.info("异步扣减库存成功，分片ID: {}, couponId={}", result, id);
            }
        });
        
        // 立即返回成功，实际结果通过其他方式通知
        return ApiResponse.success("REQUEST_ACCEPTED");
    }

    @Operation(summary = "创建补偿优惠券")
    @PostMapping("/compensation")
    public ApiResponse<Coupon> createCompensationCoupon(@RequestBody Coupon compensationCoupon) {
        log.info("创建补偿优惠券: coupon={}", compensationCoupon);
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
                return ApiResponse.success(result);
            } else {
                return ApiResponse.fail(500, "创建补偿优惠券失败");
            }
        } catch (Exception e) {
            log.error("创建补偿优惠券失败", e);
            return ApiResponse.fail(500, "创建补偿优惠券失败: " + e.getMessage());
        }
    }
}
