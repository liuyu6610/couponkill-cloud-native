package com.aliyun.seckill.couponkillgateway.security;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.utils.JwtUtil;
import io.jsonwebtoken.Claims;
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
import java.util.List;

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
        // Bypass for auth and actuator
        if (path.startsWith("/api/v1/auth/") || path.startsWith("/actuator")) {
            return chain.filter(exchange);
        }
        String auth = exchange.getRequest().getHeaders().getFirst(HttpHeaders.AUTHORIZATION);
        if (auth == null || !auth.startsWith("Bearer ")) {
            return unauthorized(exchange, "Missing or invalid Authorization");
        }
        String token = auth.substring(7);
        try {
            Claims c = JwtUtil.parse(secret, token);
            if (!issuer.equals(c.getIssuer())) {
                return unauthorized(exchange, "Invalid issuer");
            }
            // 修复点：安全地获取 audience 并判断是否包含目标 audience
            List<String> aud = (List<String>) c.get("aud");
            if (aud == null || !aud.contains(audience)) {
                return unauthorized(exchange, "Invalid audience");
            }
            String userId = c.getSubject();
            // propagate X-User-Id to downstream services
            var mutated = exchange.getRequest().mutate().header("X-User-Id", userId).build();
            return chain.filter(exchange.mutate().request(mutated).build());
        } catch (Exception e) {
            return unauthorized(exchange, "Invalid token");
        }
    }

    private Mono<Void> unauthorized(ServerWebExchange exchange, String msg){
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
    public int getOrder() { return -100; } // run early
}
