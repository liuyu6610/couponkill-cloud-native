package com.aliyun.seckill.couponkillconnectorservice.feign;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.connector.SyncStockRequest;
import com.aliyun.seckill.common.connector.SyncStockResult;
import org.springframework.cloud.openfeign.FeignClient;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestHeader;

/**
 * 直连 coupon-service（绕过网关 InternalApiBlockFilter）。
 */
@FeignClient(name = "couponkill-coupon-service", contextId = "couponStockSyncClient")
public interface CouponStockFeignClient {

    @PostMapping("/api/v1/coupon/internal/sync-stock")
    ApiResponse<SyncStockResult> syncStock(@RequestBody SyncStockRequest request,
                                           @RequestHeader("X-Internal-Token") String internalToken);
}
