package com.aliyun.seckill.couponkillgateway.security;

import org.junit.jupiter.api.Test;
import org.springframework.http.HttpMethod;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class InternalApiBlockFilterTest {

    @Test
    void blocksStockAndPreheatInternalWrites() {
        assertTrue(InternalApiBlockFilter.isBlocked("/api/v1/coupon/preheat-stock/1001", HttpMethod.POST));
        assertTrue(InternalApiBlockFilter.isBlocked("/api/v1/coupon/stock/1001", HttpMethod.POST));
        assertTrue(InternalApiBlockFilter.isBlocked("/api/v1/coupon/deduct/1001", HttpMethod.POST));
    }

    @Test
    void allowsCouponAdminWritesForJwtLayer() {
        // 方案 B：这些由 JwtAuth admin 门禁接管，不再 Internal 403
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/create", HttpMethod.POST));
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/1001/seckill-window", HttpMethod.POST));
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/1001/status", HttpMethod.POST));
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/1001", HttpMethod.DELETE));
    }

    @Test
    void allowsCouponReads() {
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/1001", HttpMethod.GET));
        assertFalse(InternalApiBlockFilter.isBlocked("/api/v1/coupon/list", HttpMethod.GET));
    }
}
