package com.aliyun.seckill.couponkillcouponservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.connector.SyncStockRequest;
import com.aliyun.seckill.common.connector.SyncStockResult;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.Parameter;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
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

    @Value("${connector.internal-token:${CONNECTOR_INTERNAL_TOKEN:couponkill-internal}}")
    private String internalToken;

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
            @Parameter(description = "秒杀总库存(仅秒抢类型有效)") @RequestParam(required = false) Integer seckillTotalStock,
            @Parameter(description = "秒杀开售时间 yyyy-MM-dd HH:mm:ss") @RequestParam(required = false)
            @org.springframework.format.annotation.DateTimeFormat(pattern = "yyyy-MM-dd HH:mm:ss") LocalDateTime seckillStartAt,
            @Parameter(description = "秒杀结束时间 yyyy-MM-dd HH:mm:ss") @RequestParam(required = false)
            @org.springframework.format.annotation.DateTimeFormat(pattern = "yyyy-MM-dd HH:mm:ss") LocalDateTime seckillEndAt) {

        log.info("创建优惠券: id={}, name={}, type={}, faceValue={}", id, name, type, faceValue);

        if (type != null && type == 2) {
            if (seckillStartAt == null || seckillEndAt == null) {
                return ApiResponse.fail(400, "秒抢券必须提供 seckillStartAt / seckillEndAt");
            }
            if (!seckillStartAt.isBefore(seckillEndAt)) {
                return ApiResponse.fail(400, "seckillStartAt 必须早于 seckillEndAt");
            }
        }

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
        coupon.setSeckillStartAt(seckillStartAt);
        coupon.setSeckillEndAt(seckillEndAt);

        // 确保必填字段有默认值
        coupon.setStatus(1); // 默认有效状态

        Coupon createdCoupon = couponService.createCoupon(coupon);
        return ApiResponse.success(createdCoupon);
    }

    @Operation(summary = "更新秒杀活动时间窗", description = "仅秒抢券；start 必须早于 end")
    @PostMapping("/{id}/seckill-window")
    public ApiResponse<Coupon> updateSeckillWindow(
            @PathVariable Long id,
            @RequestParam @org.springframework.format.annotation.DateTimeFormat(pattern = "yyyy-MM-dd HH:mm:ss")
            LocalDateTime seckillStartAt,
            @RequestParam @org.springframework.format.annotation.DateTimeFormat(pattern = "yyyy-MM-dd HH:mm:ss")
            LocalDateTime seckillEndAt) {
        try {
            return ApiResponse.success(couponService.updateSeckillWindow(id, seckillStartAt, seckillEndAt));
        } catch (IllegalArgumentException e) {
            return ApiResponse.fail(400, e.getMessage());
        }
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

    @Operation(summary = "按分片回补秒杀库存", description = "virtualId 格式 couponId_shardIndex，仅服务间调用")
    @PostMapping("/increase-seckill-by-shard")
    public ApiResponse<Boolean> increaseSeckillStockByShardId(
            @Parameter(description = "虚拟分片ID") @RequestParam String virtualId) {
        log.info("按分片回补秒杀库存: virtualId={}", virtualId);
        try {
            boolean result = couponService.increaseSeckillStockByShardId(virtualId);
            return ApiResponse.success(result);
        } catch (Exception e) {
            log.error("按分片回补秒杀库存失败，virtualId: {}", virtualId, e);
            return ApiResponse.fail(500, "按分片回补秒杀库存失败: " + e.getMessage());
        }
    }

    @Operation(summary = "预热单券 Redis 库存", description = "内部接口：秒杀缺 key 时补救")
    @PostMapping("/preheat-stock/{id}")
    public ApiResponse<Boolean> preheatStock(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        return ApiResponse.success(couponService.preheatCouponStock(id));
    }

    @Operation(summary = "全量预热 Redis 库存", description = "内部接口")
    @PostMapping("/preheat-stock")
    public ApiResponse<Boolean> preheatAllStock() {
        couponService.asyncPreheatStockToRedis();
        return ApiResponse.success(true);
    }

    @Operation(summary = "电商 Connector 同步 Redis 库存", description = "内部接口，禁止经网关暴露")
    @PostMapping("/internal/sync-stock")
    public ApiResponse<SyncStockResult> syncStockFromConnector(
            @RequestBody SyncStockRequest request,
            @RequestHeader(value = "X-Internal-Token", required = false) String token) {
        if (internalToken == null || internalToken.isBlank() || !internalToken.equals(token)) {
            return ApiResponse.fail(403, "forbidden: invalid internal token");
        }
        if (request == null || request.getCouponId() == null || request.getTargetStock() == null) {
            return ApiResponse.fail(400, "couponId/targetStock 必填");
        }
        boolean force = Boolean.TRUE.equals(request.getForce());
        SyncStockResult result = couponService.syncRedisStock(request.getCouponId(), request.getTargetStock(), force);
        if (result != null && result.isSuccess()) {
            return ApiResponse.success(result);
        }
        String msg = result != null && result.getMessage() != null ? result.getMessage() : "同步 Redis 库存失败";
        return ApiResponse.fail(500, msg);
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

    @Operation(summary = "仅扣 DB 分片秒杀库存（Redis 已由热路径 Lua 预扣）", description = "内部接口，禁止经网关暴露给客户端")
    @PostMapping("/deduct-db-only/{id}")
    public ApiResponse<String> deductDbSeckillStockOnly(@Parameter(description = "优惠券ID") @PathVariable Long id) {
        log.info("仅扣 DB 秒杀库存: id={}", id);
        try {
            String shardId = couponService.deductDbSeckillStockOnly(id);
            if (shardId != null) {
                return ApiResponse.success(shardId);
            }
            return ApiResponse.fail(500, "扣减DB库存失败");
        } catch (Exception e) {
            log.error("仅扣 DB 秒杀库存失败，couponId: {}", id, e);
            return ApiResponse.fail(500, "仅扣DB库存失败: " + e.getMessage());
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