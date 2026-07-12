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

    @PostMapping("/api/v1/coupon/increase-seckill-by-shard")
    ApiResponse<Boolean> increaseSeckillStockByShardId(@RequestParam("virtualId") String virtualId);

    @PostMapping(value = "/api/v1/coupon/deduct-with-shard-id/{id}", produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<String> deductStockWithShardId(@PathVariable("id") Long id);

    /** 异步秒杀消费者专用：只扣 DB，不 DECR Redis */
    @PostMapping(value = "/api/v1/coupon/deduct-db-only/{id}", produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<String> deductDbSeckillStockOnly(@PathVariable("id") Long id);

    /** 秒杀缺 Redis 库存 key 时补救预热 */
    @PostMapping(value = "/api/v1/coupon/preheat-stock/{id}", produces = MediaType.APPLICATION_JSON_VALUE)
    @ResponseStatus(HttpStatus.OK)
    ApiResponse<Boolean> preheatStock(@PathVariable("id") Long id);

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
        public ApiResponse<Boolean> increaseSeckillStockByShardId(String virtualId) {
            log.error("调用 increaseSeckillStockByShardId 失败，virtualId: {}", virtualId);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<String> deductStockWithShardId(Long id) {
            log.error("调用 deductStockWithShardId 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<String> deductDbSeckillStockOnly(Long id) {
            log.error("调用 deductDbSeckillStockOnly 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Boolean> preheatStock(Long id) {
            log.error("调用 preheatStock 失败，couponId: {}", id);
            return ApiResponse.fail(500, "服务暂时不可用");
        }

        @Override
        public ApiResponse<Coupon> createCompensationCoupon(Coupon compensationCoupon) {
            log.error("调用 createCompensationCoupon 失败");
            return ApiResponse.fail(500, "服务暂时不可用");
        }

    }
}