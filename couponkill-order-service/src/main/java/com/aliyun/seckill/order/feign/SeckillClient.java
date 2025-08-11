package com.aliyun.seckill.order.feign;

import com.aliyun.seckill.common.api.ApiResp;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.*;

@FeignClient(name = "coupon-service", url = "${coupon-service.url:http://localhost:8080}")
public interface SeckillClient {

    @PostMapping("/api/v1/seckill/{couponId}/enter")
    ApiResp<EnterSeckillResp> enter(@PathVariable String couponId,
                                    @RequestHeader("X-User-Id") String userId);

    @GetMapping("/api/v1/seckill/results/{requestId}")
    ApiResp<SeckillResultResp> result(@PathVariable String requestId);

    @PostMapping("/api/v1/seckill/internal/success")
    ApiResp<Void> markSuccess(@RequestParam String requestId,
                              @RequestParam String orderId);

    @PostMapping("/api/v1/seckill/internal/compensate")
    ApiResp<Void> compensate(@RequestParam String requestId,
                             @RequestParam String couponId,
                             @RequestParam String userId);
}
