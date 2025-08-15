package com.aliyun.seckill.couponkillcouponservice.controller;

import com.aliyun.seckill.common.api.ApiResponse; // 修正导入
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillResultResp;
import com.aliyun.seckill.couponkillcouponservice.service.SeckillService;
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
    public ApiResponse<EnterSeckillResp> enter(@PathVariable String couponId,
                                           @RequestHeader("X-User-Id") String userId){
        var r = svc.enter(couponId, userId, cooldownSeconds, deductTtlSeconds);
        if ("QUEUED".equals(r.status())) {
            // 使用 success 方法替代 ok
            return ApiResponse.success(new EnterSeckillResp("QUEUED"));
        } else {
            int code = r.err()==0 ? ErrorCodes.INVALID_REQ : r.err();
            // 使用 fail 方法替代 err
            return ApiResponse.fail(code, "REJECTED");
        }
    }

    @GetMapping("/seckill/results/{requestId}")
    public ApiResponse<SeckillResultResp> result(@PathVariable String requestId){
        String raw = svc.getResult(requestId); // null | PENDING | SUCCESS:orderId | FAIL
        if (raw == null) return ApiResponse.success(new SeckillResultResp("PENDING", null));
        if (raw.startsWith("SUCCESS:")) return ApiResponse.success(new SeckillResultResp("SUCCESS", raw.substring(8)));
        if ("FAIL".equals(raw)) return ApiResponse.success(new SeckillResultResp("FAIL", null));
        return ApiResponse.success(new SeckillResultResp("PENDING", null));
    }

    // 给 order-service 回调用（内部调用或事件触发）
    @PostMapping("/seckill/internal/success")
    public ApiResponse<Void> markSuccess(@RequestParam String requestId, @RequestParam String orderId){
        svc.markSuccess(requestId, orderId);
        return ApiResponse.success(null);
    }

    @PostMapping("/seckill/internal/compensate")
    public ApiResponse<Void> compensate(@RequestParam String requestId, @RequestParam String couponId, @RequestParam String userId){
        svc.compensateFail(requestId, couponId, userId);
        return ApiResponse.success(null);
    }
}
