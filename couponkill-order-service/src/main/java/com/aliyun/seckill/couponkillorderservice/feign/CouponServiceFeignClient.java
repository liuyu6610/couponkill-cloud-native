// 创建包: com.aliyun.seckill.couponkillorderservice.feign
package com.aliyun.seckill.couponkillorderservice.feign;

import com.aliyun.seckill.common.api.ApiResp;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.*;

@FeignClient(name = "coupon-service")
public interface CouponServiceFeignClient {

    @GetMapping("/coupon/{id}")
    Coupon getCouponById(@PathVariable("id") Long id);

    @PostMapping("/coupon/deduct/{id}")
    boolean deductStock(@PathVariable("id") Long id);

    @PostMapping("/coupon/increase/{id}")
    boolean increaseStock(@PathVariable("id") Long id);

    // 添加秒杀相关接口
    @PostMapping("/api/v1/seckill/{couponId}/enter")
    ApiResp<EnterSeckillResp> enter(@PathVariable("couponId") String couponId,
                                    @RequestHeader("X-User-Id") String userId);

    @GetMapping("/api/v1/seckill/results/{requestId}")
    ApiResp<SeckillResultResp> result(@PathVariable("requestId") String requestId);

    @PostMapping("/api/v1/seckill/internal/success")
    ApiResp<Void> markSuccess(@RequestParam("requestId") String requestId,
                              @RequestParam("orderId") String orderId);

    @PostMapping("/api/v1/seckill/internal/compensate")
    ApiResp<Void> compensate(@RequestParam("requestId") String requestId,
                             @RequestParam("couponId") String couponId,
                             @RequestParam("userId") String userId);
}
