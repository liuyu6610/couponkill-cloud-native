// 文件路径: com/aliyun/seckill/couponkillgateway/security/JwtAuthGlobalFilter.java
package com.aliyun.seckill.couponkillgateway.security;

import com.aliyun.seckill.couponkillgateway.api.ErrorCodes;
import com.aliyun.seckill.couponkillgateway.utils.JwtUtil;
import io.jsonwebtoken.Claims;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cloud.gateway.filter.GatewayFilterChain;
import org.springframework.cloud.gateway.filter.GlobalFilter;
import org.springframework.core.Ordered;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;

import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.List;

@Slf4j
@Component
public class JwtAuthGlobalFilter implements GlobalFilter, Ordered {

    @Value("${auth.jwt.secret:CHANGE_ME_256bit_secret_please_CHANGE_ME_1234567890}")
    private String secret;

    @Value("${auth.jwt.issuer:https://auth.couponkill}")
    private String issuer;

    @Value("${auth.jwt.audience:couponkill}")
    private String audience;

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        String path = exchange.getRequest().getURI().getPath();
        log.info("Request path: {}", path);

        // 放行不需要认证的路径
        if (path.startsWith("/api/v1/auth/")
                || path.startsWith("/actuator")
                || path.startsWith("/user/register")
                || path.startsWith("/user/login")
                || path.startsWith("/doc.html")
                || path.startsWith("/swagger-ui")
                || path.startsWith("/v3/api-docs")) {
            log.info("Bypassing authentication for path: {}", path);
            return chain.filter(exchange);
        }

        String auth = exchange.getRequest().getHeaders().getFirst(HttpHeaders.AUTHORIZATION);
        if (auth == null || !auth.startsWith("Bearer ")) {
            return unauthorized(exchange, "Missing or invalid Authorization");
        }

        String token = auth.substring(7);
        try {
            Claims claims = JwtUtil.parse(secret, token);

            // 验证issuer
            if (!issuer.equals(claims.getIssuer())) {
                return unauthorized(exchange, "Invalid issuer");
            }

            // 验证audience
            Object audObj = claims.get("aud");
            List<String> aud;
            if (audObj instanceof String) {
                aud = Collections.singletonList((String) audObj);
            } else if (audObj instanceof List) {
                aud = (List<String>) audObj;
            } else {
                return unauthorized(exchange, "Invalid audience");
            }

            if (!aud.contains(audience)) {
                return unauthorized(exchange, "Invalid audience");
            }

            String userId = claims.getSubject();
            Object rolesObj = claims.get("roles");
            String rolesStr = "";

            if (rolesObj instanceof List) {
                List<String> roles = (List<String>) rolesObj;
                rolesStr = String.join(",", roles);
            } else if (rolesObj instanceof String) {
                rolesStr = (String) rolesObj;
            }

            // 传递用户信息到下游服务
            var mutated = exchange.getRequest().mutate()
                    .header("X-User-Id", userId)
                    .header("X-User-Roles", rolesStr)
                    .header("X-Authenticated", "true")
                    .build();

            return chain.filter(exchange.mutate().request(mutated).build());
        } catch (Exception e) {
            log.error("Token validation error", e);
            return unauthorized(exchange, "Invalid token");
        }
    }

    private Mono<Void> unauthorized(ServerWebExchange exchange, String msg) {
        exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
        exchange.getResponse().getHeaders().setContentType(MediaType.APPLICATION_JSON);
        var body = ("{" +
                "\"code\": " + ErrorCodes.AUTH_FAIL + "," +
                "\"message\": \"" + msg + "\"," +
                "\"data\": null}"
        ).getBytes(StandardCharsets.UTF_8);
        return exchange.getResponse().writeWith(Mono.just(exchange.getResponse().bufferFactory().wrap(body)));
    }

    @Override
    public int getOrder() {
        return -100; // 确保在其他过滤器之前执行
    }
}
