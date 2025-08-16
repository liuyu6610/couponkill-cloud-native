package com.aliyun.seckill.couponkillgateway.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jws;
import io.jsonwebtoken.Jwts;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import javax.annotation.PostConstruct;
import javax.crypto.spec.SecretKeySpec;
import java.security.Key;
import java.util.Base64;

@Component
public class JwtUtil {

    @Value("${jwt.secret:secret}")
    private String secret;

    private String encodedSecret;

    @PostConstruct
    public void init() {
        this.encodedSecret = Base64.getEncoder().encodeToString(secret.getBytes());
    }

    public static Claims parse(String secret, String token) {
        Key signingKey = new SecretKeySpec(
                Base64.getDecoder().decode(secret),
                "HmacSHA256"
        );

        Jws<Claims> claimsJws = Jwts.parserBuilder()
                .setSigningKey(signingKey)
                .build()
                .parseClaimsJws(token);

        return claimsJws.getBody();
    }

    public boolean verifyToken(String token) {
        try {
            return parse(encodedSecret, token) != null;
        } catch (Exception e) {
            return false;
        }
    }

    public String getUserId(String token) {
        // 获取userId并转换为String类型
        Object userIdObj = parse(encodedSecret, token).get("userId");
        if (userIdObj instanceof Integer) {
            return String.valueOf(userIdObj);
        } else if (userIdObj instanceof String) {
            return (String) userIdObj;
        } else {
            throw new IllegalArgumentException("userId claim is not of expected type");
        }
    }
}
