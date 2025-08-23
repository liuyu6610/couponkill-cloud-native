package com.aliyun.seckill.couponkillgateway.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jws;
import io.jsonwebtoken.Jwts;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import javax.annotation.PostConstruct;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.Key;
import java.util.Base64;
@Slf4j
@Component
public class JwtUtil {

    @Value("${jwt.secret:secret}")
    private String secret;

    private String encodedSecret;

    @PostConstruct
    public void init() {
        // 确保密钥正确编码
        this.encodedSecret = Base64.getEncoder().encodeToString(secret.getBytes(StandardCharsets.UTF_8));
    }

    public Claims parse(String secret, String token) {
        try {
            Key signingKey = new SecretKeySpec(
                    Base64.getDecoder().decode(secret),
                    "HmacSHA256"
            );

            Jws<Claims> claimsJws = Jwts.parserBuilder()
                    .setSigningKey(signingKey)
                    .build()
                    .parseClaimsJws(token);

            return claimsJws.getBody();
        } catch (Exception e) {
            // 记录详细错误信息便于排查
            log.warn("JWT解析失败: token={}, error={}", token, e.getMessage());
            throw e; // 重新抛出异常
        }
    }

    public boolean verifyToken(String token) {
        if (token == null || token.isEmpty()) {
            return false;
        }

        try {
            return parse(encodedSecret, token) != null;
        } catch (Exception e) {
            log.debug("Token验证失败: {}", e.getMessage());
            return false;
        }
    }

    public String getUserId(String token) {
        try {
            Object userIdObj = parse(encodedSecret, token).get("userId");
            if (userIdObj == null) {
                throw new IllegalArgumentException("Token中不包含userId");
            }

            if (userIdObj instanceof Integer) {
                return String.valueOf(userIdObj);
            } else if (userIdObj instanceof String) {
                return (String) userIdObj;
            } else if (userIdObj instanceof Long) {
                return String.valueOf(userIdObj);
            } else {
                throw new IllegalArgumentException("userId claim类型不正确: " + userIdObj.getClass());
            }
        } catch (Exception e) {
            log.warn("获取userId失败: token={}, error={}", token, e.getMessage());
            throw e;
        }
    }
}

