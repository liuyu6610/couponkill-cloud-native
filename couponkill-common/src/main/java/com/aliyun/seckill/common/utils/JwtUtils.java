// com.aliyun.seckill.common.utils.JwtUtils.java
package com.aliyun.seckill.common.utils;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.SignatureAlgorithm;
import io.jsonwebtoken.security.Keys;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.security.Key;
import java.util.*;
import java.util.function.Function;
import java.util.UUID;

@Component
public class JwtUtils {

    @Value("${jwt.secret:myDefaultSecretKeyForJWTTokenGenerationWhichShouldBeAtLeast256BitsLong}")
    private String secret;

    @Value("${jwt.expiration:86400000}")
    private long expiration;

    @Value("${auth.jwt.issuer:https://auth.couponkill}")
    private String issuer;

    @Value("${auth.jwt.audience:couponkill}")
    private String audience;

    private Key getSigningKey() {
        return Keys.hmacShaKeyFor(secret.getBytes());
    }

    // 生成令牌 (原有方法)
    public String generateToken(Long userId) {
        Map<String, Object> claims = new HashMap<>();
        claims.put("userId", userId);
        return createToken(claims);
    }

    private String createToken(Map<String, Object> claims) {
        return Jwts.builder()
                .setClaims(claims)
                .setIssuedAt(new Date(System.currentTimeMillis()))
                .setExpiration(new Date(System.currentTimeMillis() + expiration))
                .signWith(getSigningKey(), SignatureAlgorithm.HS256)
                .compact();
    }

    // 新增：支持创建带角色的token
    public String createToken(String userId, List<String> roles, long ttlSeconds) {
        long now = System.currentTimeMillis();
        Date iat = new Date(now);
        Date exp = new Date(now + ttlSeconds * 1000);

        return Jwts.builder()
                .setHeaderParam("kid", UUID.randomUUID().toString())
                .setSubject(userId)
                .setIssuer(issuer)
                .setAudience(audience)
                .claim("roles", roles)
                .setIssuedAt(iat)
                .setExpiration(exp)
                .signWith(getSigningKey(), SignatureAlgorithm.HS256)
                .compact();
    }

    // 从令牌中获取用户ID
    public Long extractUserId(String token) {
        return extractClaim(token, claims -> claims.get("userId", Long.class));
    }

    // 从令牌中获取声明
    public <T> T extractClaim(String token, Function<Claims, T> claimsResolver) {
        final Claims claims = extractAllClaims(token);
        return claimsResolver.apply(claims);
    }

    private Claims extractAllClaims(String token) {
        return Jwts.parserBuilder()
                .setSigningKey(getSigningKey())
                .build()
                .parseClaimsJws(token)
                .getBody();
    }

    // 验证令牌是否过期
    public boolean isTokenExpired(String token) {
        return extractExpiration(token).before(new Date());
    }

    private Date extractExpiration(String token) {
        return extractClaim(token, Claims::getExpiration);
    }

    // 验证令牌
    public boolean validateToken(String token, Long userId) {
        final Long extractedUserId = extractUserId(token);
        return (extractedUserId.equals(userId) && !isTokenExpired(token));
    }

    // 解析token获取Claims
    public Claims parse(String token) {
        return Jwts.parserBuilder().setSigningKey(getSigningKey()).build().parseClaimsJws(token).getBody();
    }

    // 提取用户ID (从Subject)
    public String extractSubject(String token) {
        return extractClaim(token, Claims::getSubject);
    }

    // 验证token通用方法
    public boolean validateToken(String token, String userId) {
        try {
            Claims claims = parse(token);
            return claims.getSubject().equals(userId) && !isTokenExpired(token);
        } catch (Exception e) {
            return false;
        }
    }
}
