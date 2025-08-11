package com.aliyun.seckill.coupon.controller;

import com.aliyun.seckill.common.api.ApiResp;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import com.aliyun.seckill.coupon.service.SeckillService;
import lombok.RequiredArgsConstructor;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/v1")
@RequiredArgsConstructor
public class SeckillController {
    private final SeckillService svc;
    @Value("${couponkill.seckill.cooldown-seconds}")
    private int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds}")
    private int deductTtlSeconds;
    @PostMapping("/seckill/{couponId}/enter")
    public ApiResp<EnterSeckillResp> enter(@PathVariable String couponId,
                                           @RequestHeader("X-User-Id") String userId){
        var r = svc.enter(couponId, userId, cooldownSeconds, deductTtlSeconds);
        if ("QUEUED".equals(r.status())) {
            return ApiResp.ok(new EnterSeckillResp("QUEUED"), r.requestId());
        } else {
            int code = r.err()==0 ? ErrorCodes.INVALID_REQ : r.err();
            return ApiResp.err(code, "REJECTED", r.requestId());
        }
    }

    @GetMapping("/seckill/results/{requestId}")
    public ApiResp<SeckillResultResp> result(@PathVariable String requestId){
        String raw = svc.getResult(requestId); // null | PENDING | SUCCESS:orderId | FAIL
        if (raw == null) return ApiResp.ok(new SeckillResultResp("PENDING", null), requestId);
        if (raw.startsWith("SUCCESS:")) return ApiResp.ok(new SeckillResultResp("SUCCESS", raw.substring(8)), requestId);
        if ("FAIL".equals(raw)) return ApiResp.ok(new SeckillResultResp("FAIL", null), requestId);
        return ApiResp.ok(new SeckillResultResp("PENDING", null), requestId);
    }

    // 给 order-service 回调用（内部调用或事件触发）
    @PostMapping("/seckill/internal/success")
    public ApiResp<Void> markSuccess(@RequestParam String requestId, @RequestParam String orderId){
        svc.markSuccess(requestId, orderId);
        return ApiResp.ok(null, requestId);
    }

    @PostMapping("/seckill/internal/compensate")
    public ApiResp<Void> compensate(@RequestParam String requestId, @RequestParam String couponId, @RequestParam String userId){
        svc.compensateFail(requestId, couponId, userId);
        return ApiResp.ok(null, requestId);
    }
}
