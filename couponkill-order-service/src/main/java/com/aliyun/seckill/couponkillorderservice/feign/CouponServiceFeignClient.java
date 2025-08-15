package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.*;

// 修复CouponServiceFeignClient.java
@FeignClient(name = "coupon-service")
public interface CouponServiceFeignClient {

    @GetMapping("/api/v1/coupon/{id}")
    Coupon getCouponById(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/deduct/{id}")
    boolean deductStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/increase/{id}")
    boolean increaseStock(@PathVariable("id") Long id);

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
    void createCompensationCoupon(@RequestBody Coupon compensationCoupon);

    // 添加缺失的库存锁定相关方法
    @PostMapping("/api/v1/coupon/lock/{id}")
    boolean lockStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/confirm/{id}")
    boolean confirmDeductStock(@PathVariable("id") Long id);

    @PostMapping("/api/v1/coupon/release/{id}")
    boolean releaseStock(@PathVariable("id") Long id);
}
