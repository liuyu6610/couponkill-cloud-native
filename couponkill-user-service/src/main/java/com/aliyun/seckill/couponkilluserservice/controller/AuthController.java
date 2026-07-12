package com.aliyun.seckill.couponkilluserservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import io.jsonwebtoken.SignatureAlgorithm;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import lombok.Data;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.nio.charset.StandardCharsets;
import java.util.Date;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Mock token 签发端。claim 与正式登录一致：含 userId；密钥统一走 jwt.secret。
 */
@RestController
@RequestMapping("/api/v1/auth")
public class AuthController {

    @Value("${jwt.secret:mySecretKeyForJWTTokenGenerationWhichShouldBeAtLeast256BitsLong}")
    private String secret;
    @Value("${auth.jwt.issuer:https://auth.couponkill}")
    private String issuer;
    @Value("${auth.jwt.audience:couponkill}")
    private String audience;
    @Value("${auth.jwt.mock-ttl-seconds:3600}")
    private long ttlSeconds;

    @PostMapping("/token/mock")
    public ApiResponse<TokenResp> mock(@RequestBody MockReq req) {
        if (req.getUserId() == null || req.getUserId().isBlank()) {
            return ApiResponse.fail(400, "userId不能为空");
        }
        List<String> roles = req.getRoles() == null ? List.of("user") : req.getRoles();

        Map<String, Object> claims = new HashMap<>();
        claims.put("userId", req.getUserId());
        claims.put("roles", roles);

        String token = Jwts.builder()
                .setClaims(claims)
                .setSubject(req.getUserId())
                .setIssuer(issuer)
                .setAudience(audience)
                .setIssuedAt(new Date())
                .setExpiration(new Date(System.currentTimeMillis() + ttlSeconds * 1000))
                .signWith(Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8)), SignatureAlgorithm.HS256)
                .compact();

        TokenResp resp = new TokenResp();
        resp.setToken(token);
        resp.setTokenType("Bearer");
        resp.setExpiresIn(ttlSeconds);
        return ApiResponse.success(resp);
    }

    @Data
    public static class MockReq {
        private String userId;
        private List<String> roles;
    }

    @Data
    public static class TokenResp {
        private String tokenType;
        private String token;
        private long expiresIn;
    }
}
