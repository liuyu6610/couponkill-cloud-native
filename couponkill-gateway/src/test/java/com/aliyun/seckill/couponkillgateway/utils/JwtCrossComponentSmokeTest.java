package com.aliyun.seckill.couponkillgateway.utils;

import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.SignatureAlgorithm;
import io.jsonwebtoken.security.Keys;
import org.junit.jupiter.api.Test;
import org.springframework.test.util.ReflectionTestUtils;

import java.nio.charset.StandardCharsets;
import java.util.Date;
import java.util.List;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * JWT 跨组件冒烟：按 user 侧 JwtUtils.createToken 同款算法签发，gateway JwtUtil 验签。
 * 不依赖 Docker 全栈；HTTP 联调需本地起 gateway/user。
 */
class JwtCrossComponentSmokeTest {

    private static final String SECRET =
            "mySecretKeyForJWTTokenGenerationWhichShouldBeAtLeast256BitsLong";

    @Test
    void loginStyleToken_gatewayCanVerifyAndReadUserId() {
        long now = System.currentTimeMillis();
        String token = Jwts.builder()
                .setHeaderParam("kid", UUID.randomUUID().toString())
                .setSubject("10000")
                .setIssuer("https://auth.couponkill")
                .setAudience("couponkill")
                .claim("userId", "10000")
                .claim("roles", List.of("user", "admin"))
                .setIssuedAt(new Date(now))
                .setExpiration(new Date(now + 3600_000))
                .signWith(Keys.hmacShaKeyFor(SECRET.getBytes(StandardCharsets.UTF_8)), SignatureAlgorithm.HS256)
                .compact();

        JwtUtil jwtUtil = new JwtUtil();
        ReflectionTestUtils.setField(jwtUtil, "secret", SECRET);

        assertTrue(jwtUtil.verifyToken(token));
        assertEquals("10000", jwtUtil.getUserId(token));
        assertEquals(List.of("user", "admin"), jwtUtil.getRoles(token));
    }
}
