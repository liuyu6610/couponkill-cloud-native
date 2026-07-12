package com.aliyun.seckill.couponkillgateway.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.security.Key;
import java.util.List;

/**
 * 网关 JWT 工具。密钥算法与签发端 JwtUtils 保持一致：Keys.hmacShaKeyFor(rawSecretBytes)。
 */
@Slf4j
@Component
public class JwtUtil {

    @Value("${jwt.secret:mySecretKeyForJWTTokenGenerationWhichShouldBeAtLeast256BitsLong}")
    private String secret;

    private Key getSigningKey() {
        return Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
    }

    public Claims parse(String token) {
        return Jwts.parserBuilder()
                .setSigningKey(getSigningKey())
                .build()
                .parseClaimsJws(token)
                .getBody();
    }

    public boolean verifyToken(String token) {
        if (token == null || token.isEmpty()) {
            return false;
        }
        try {
            return parse(token) != null;
        } catch (Exception e) {
            log.debug("Token验证失败: {}", e.getMessage());
            return false;
        }
    }

    /**
     * 优先读 claim userId（登录签发），兼容 subject（mock token）。
     */
    public String getUserId(String token) {
        try {
            Claims claims = parse(token);
            Object userIdObj = claims.get("userId");
            if (userIdObj != null) {
                return String.valueOf(userIdObj);
            }
            String subject = claims.getSubject();
            if (subject != null && !subject.isEmpty()) {
                return subject;
            }
            throw new IllegalArgumentException("Token中不包含userId/subject");
        } catch (Exception e) {
            log.warn("获取userId失败: error={}", e.getMessage());
            throw e;
        }
    }

    @SuppressWarnings("unchecked")
    public List<String> getRoles(String token) {
        Claims claims = parse(token);
        Object roles = claims.get("roles");
        if (roles instanceof List<?> list) {
            return list.stream().map(String::valueOf).toList();
        }
        return List.of("user");
    }
}
