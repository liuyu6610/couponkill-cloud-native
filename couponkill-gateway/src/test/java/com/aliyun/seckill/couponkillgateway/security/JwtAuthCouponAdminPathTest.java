package com.aliyun.seckill.couponkillgateway.security;

import org.junit.jupiter.api.Test;
import org.springframework.http.HttpMethod;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class JwtAuthCouponAdminPathTest {

    @Test
    void couponAdminWritesRequireAdminGate() {
        assertTrue(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/create", HttpMethod.POST));
        assertTrue(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/1001/seckill-window", HttpMethod.POST));
        assertTrue(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/1001/status", HttpMethod.POST));
        assertTrue(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/1001", HttpMethod.DELETE));
    }

    @Test
    void couponReadsAndInternalStockNotAdminGate() {
        assertFalse(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/1001", HttpMethod.GET));
        assertFalse(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/list", HttpMethod.GET));
        assertFalse(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/preheat-stock/1001", HttpMethod.POST));
        assertFalse(JwtAuthGlobalFilter.isCouponAdminPath("/api/v1/coupon/stock/1001", HttpMethod.POST));
    }
}
