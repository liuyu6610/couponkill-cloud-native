package com.aliyun.seckill.couponkillgateway.security;

import com.aliyun.seckill.couponkillgateway.utils.JwtUtil;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.cloud.gateway.filter.GatewayFilterChain;
import org.springframework.cloud.gateway.filter.GlobalFilter;
import org.springframework.core.Ordered;
import org.springframework.http.HttpStatus;
import org.springframework.http.server.reactive.ServerHttpRequest;
import org.springframework.stereotype.Component;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;

import java.util.List;

@Slf4j
@Component
public class JwtAuthGlobalFilter implements GlobalFilter, Ordered {

    // 不需要认证的路径前缀
    private static final List<String> WHITE_LIST = List.of(
            "/fallback/",              // 网关降级接口
            "/api/v1/user/register",   // 用户注册接口
            "/api/v1/user/login" ,// 用户登录接口
            "/api/v1/auth/token/mock"
    );

    @Autowired
    private JwtUtil jwtUtil;

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        ServerHttpRequest request = exchange.getRequest();
        String path = request.getURI().getPath();

        // 检查是否在白名单中
        if (isWhitelisted(path)) {
            return chain.filter(exchange);
        }

        // 对于需要认证的接口进行JWT验证
        String token = extractToken(request);
        if (token == null || !jwtUtil.verifyToken(token)) {
            exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
            return exchange.getResponse().setComplete();
        }

        // 将用户ID存入请求头
        String userId = jwtUtil.getUserId(token);
        ServerHttpRequest mutatedRequest = request.mutate()
                .header("X-User-ID", userId)
                .build();
        ServerWebExchange mutatedExchange = exchange.mutate().request(mutatedRequest).build();

        return chain.filter(mutatedExchange);
    }

    private boolean isWhitelisted(String path) {
        return WHITE_LIST.stream().anyMatch(path::startsWith);
    }

    private String extractToken(ServerHttpRequest request) {
        String authHeader = request.getHeaders().getFirst("Authorization");
        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            return authHeader.substring(7);
        }
        return null;
    }

    @Override
    public int getOrder() {
        return -100; // 确保在路由前执行
    }
}
