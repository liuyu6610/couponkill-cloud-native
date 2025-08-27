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

    private static final List<String> WHITE_LIST = List.of(
            "/fallback/",
            "/api/v1/user/register",
            "/api/v1/user/login",
            "/api/v1/auth/token/mock"
    );

    @Autowired
    private JwtUtil jwtUtil;

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        ServerHttpRequest request = exchange.getRequest();
        String path = request.getURI().getPath();

        log.debug("收到请求: path={}", path);

        // 检查是否在白名单中
        if (isWhitelisted(path)) {
            log.debug("路径在白名单中，跳过认证: path={}", path);
            return chain.filter(exchange);
        }

        // 对于需要认证的接口进行JWT验证
        String token = extractToken(request);
        if (token == null || token.isEmpty()) {
            log.warn("请求缺少认证token: path={}", path);
            exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
            return exchange.getResponse().setComplete();
        }

        try {
            if (!jwtUtil.verifyToken(token)) {
                log.warn("Token验证失败: token={}, path={}", token, path);
                exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
                return exchange.getResponse().setComplete();
            }

            // 将用户ID存入请求头
            String userId = jwtUtil.getUserId(token);
            if (userId == null || userId.isEmpty()) {
                log.warn("Token中userId为空: token={}, path={}", token, path);
                exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
                return exchange.getResponse().setComplete();
            }

            log.debug("Token验证成功，userId={}", userId);

            ServerHttpRequest mutatedRequest = request.mutate()
                    .header("X-User-ID", userId)
                    .build();
            ServerWebExchange mutatedExchange = exchange.mutate().request(mutatedRequest).build();

            return chain.filter(mutatedExchange);
        } catch (Exception e) {
            log.error("认证过程出现异常: path={}, error={}", path, e.getMessage(), e);
            exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
            return exchange.getResponse().setComplete();
        }
    }

    private boolean isWhitelisted(String path) {
        return WHITE_LIST.stream().anyMatch(path::startsWith);
    }

    private String extractToken(ServerHttpRequest request) {
        String authHeader = request.getHeaders().getFirst("Authorization");
        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            return authHeader.substring(7).trim();
        }
        return null;
    }

    @Override
    public int getOrder() {
        return -100; // 确保在路由前执行
    }
}

