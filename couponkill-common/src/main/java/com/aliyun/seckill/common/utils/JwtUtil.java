package com.aliyun.seckill.common.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import java.nio.charset.StandardCharsets;
import java.security.Key;
import java.util.*;

public class JwtUtil {
    // In prod, inject via env/secret; here keep default for testing
    public static String createToken(String secret, String issuer, String audience, String userId, List<String> roles, long ttlSeconds){
        Key key = Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
        long now = System.currentTimeMillis();
        Date iat = new Date(now);
        Date exp = new Date(now + ttlSeconds * 1000);
        return Jwts.builder()
                .setHeaderParam("kid", UUID.randomUUID().toString()) // 使用setHeaderParam替代.id()
                .setSubject(userId)
                .setIssuer(issuer)
                .setAudience(audience)
                .claim("roles", roles)
                .setIssuedAt(iat)
                .setExpiration(exp)
                .signWith(key, io.jsonwebtoken.SignatureAlgorithm.HS256)
                .compact();
    }

    public static Claims parse(String secret, String token){
        Key key = Keys.hmacShaKeyFor(secret.getBytes(StandardCharsets.UTF_8));
        return Jwts.parserBuilder().setSigningKey(key).build().parseClaimsJws(token).getBody();
    }
}
