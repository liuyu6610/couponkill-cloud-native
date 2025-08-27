// 修改 CouponServiceFeignClient.java
package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.config.FeignConfig;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import lombok.extern.slf4j.Slf4j;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.bind.annotation.*;

// 在CouponServiceFeignClient中优化配置
@FeignClient(
        name = "couponkill-coupon-service",
        configuration = FeignConfig.class,
        fallback = CouponServiceFeignClient.CouponServiceFallback.class
)
public interface CouponServiceFeignClient {

    @GetMapping(value = "/api/v1/coupon/{id}",
            produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<Coupon> getCouponById(@PathVariable("id") Long id);
    @PostMapping("/api/v1/coupon/deduct/{id}")
    ApiResponse<Boolean> deductStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/increase/{id}")
    ApiResponse<Boolean> increaseStock(@PathVariable("id") Long id);
    @PostMapping("/api/v1/coupon/deduct-with-virtual-id/{id}")
    ApiResponse<String> deductStockWithVirtualId(@PathVariable("id") Long id);
    // 添加秒杀相关接口
    @PostMapping("/api/v1/seckill/{couponId}/enter")
    ApiResponse<EnterSeckillResp> enter(@PathVariable("couponId") String couponId,
                                        @RequestHeader("X-User-Id") String userId);

    @GetMapping("/api/v1/seckill/results/{requestId}")
    ApiResponse<SeckillResultResp> result(@PathVariable("requestId") String requestId);

    @PostMapping("/api/v1/seckill/internal/success")
    ApiResponse<Void> markSuccess(@RequestParam("requestId") String requestId,
                              @RequestParam("orderId") String orderId);

    @PostMapping("/api/v1/seckill/internal/compensate")
    ApiResponse<Void> compensate(@RequestParam("requestId") String requestId,
                             @RequestParam("couponId") String couponId,
                             @RequestParam("userId") String userId);

    // 添加缺失的方法注解
    @PostMapping("/api/v1/coupon/compensation")
    ApiResponse<Void> createCompensationCoupon(@RequestBody Coupon compensationCoupon);

    // 添加缺失的库存锁定相关方法
    @PostMapping("/api/v1/coupon/lock/{id}")
    ApiResponse<Boolean> lockStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/confirm/{id}")
    ApiResponse<Boolean> confirmDeductStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/release/{id}")
    ApiResponse<Boolean> releaseStock(@PathVariable("id") Long id);
    @PostMapping("/api/v1/coupon/sync-stock/{id}")
    ApiResponse<Boolean> syncStock(@PathVariable("id") Long id);

    @Component
    @Slf4j
    class CouponServiceFallback implements CouponServiceFeignClient {

        @Override
        public ApiResponse<Coupon> getCouponById(Long id) {
            log.error("调用 getCouponById 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> deductStock(Long id) {
            log.error("调用 deductStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> increaseStock(Long id) {
            log.error("调用 increaseStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<String> deductStockWithVirtualId(Long id) {
            log.error("调用 deductStockWithVirtualId 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<EnterSeckillResp> enter(String couponId, String userId) {
            log.error("调用 enter 失败，couponId: {}, userId: {}", couponId, userId);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<SeckillResultResp> result(String requestId) {
            log.error("调用 result 失败，requestId: {}", requestId);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Void> markSuccess(String requestId, String orderId) {
            log.error("调用 markSuccess 失败，requestId: {}, orderId: {}", requestId, orderId);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Void> compensate(String requestId, String couponId, String userId) {
            log.error("调用 compensate 失败，requestId: {}, couponId: {}, userId: {}", requestId, couponId, userId);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Void> createCompensationCoupon(Coupon compensationCoupon) {
            log.error("调用 createCompensationCoupon 失败");
            return null;
        }

        @Override
        public ApiResponse<Boolean> lockStock(Long id) {
            log.error("调用 lockStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> confirmDeductStock(Long id) {
            log.error("调用 confirmDeductStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> releaseStock(Long id) {
            log.error("调用 releaseStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> syncStock(Long id) {
            return null;
        }
    }
}
