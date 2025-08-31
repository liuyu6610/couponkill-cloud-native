// 修改 CouponServiceFeignClient.java
package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.config.FeignConfig;
import com.aliyun.seckill.common.pojo.Coupon;
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

    @PostMapping(value = "/api/v1/coupon/deduct-with-shard-id/{id}", produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<String> deductStockWithShardId(@PathVariable("id") Long id);

    @PostMapping(value = "/api/v1/coupon/compensation", produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<Coupon> createCompensationCoupon(@RequestBody Coupon compensationCoupon);

    @Slf4j
    @Component
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
        public ApiResponse<String> deductStockWithShardId(Long id) {
            log.error("调用 deductStockWithShardId 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Coupon> createCompensationCoupon(Coupon compensationCoupon) {
            log.error("调用 createCompensationCoupon 失败");
            return ApiResponse.fail(500, "服务暂时不可用");
        }

    }
}