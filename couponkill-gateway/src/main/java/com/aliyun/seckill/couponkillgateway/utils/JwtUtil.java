package com.aliyun.seckill.couponkillgateway.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jws;
import io.jsonwebtoken.Jwts;

import javax.crypto.spec.SecretKeySpec;
import java.security.Key;
import java.util.Base64;

public class JwtUtil {
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
}
