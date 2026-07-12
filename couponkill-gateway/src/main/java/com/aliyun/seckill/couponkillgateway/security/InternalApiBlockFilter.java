package com.aliyun.seckill.couponkillgateway.security;

import lombok.extern.slf4j.Slf4j;
import org.springframework.cloud.gateway.filter.GatewayFilterChain;
import org.springframework.cloud.gateway.filter.GlobalFilter;
import org.springframework.core.Ordered;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;

import java.nio.charset.StandardCharsets;
import java.util.List;

/**
 * 阻断仅供服务间调用的内部写接口从网关暴露（库存扣减、用户券计数改写等）。
 * 这些接口应由集群内 Feign/服务发现直连，不得经公网网关。
 */
@Slf4j
@Component
public class InternalApiBlockFilter implements GlobalFilter, Ordered {

    private static final List<String> BLOCKED_PREFIXES = List.of(
            "/api/v1/user/coupon/",
            "/api/v1/user/batch/",
            "/api/v1/coupon/stock/",
            "/api/v1/coupon/create",
            "/order/admin"
    );

    private static final List<String> BLOCKED_CONTAINS = List.of(
            "/deduct/",
            "/increase/",
            "/increase-seckill-by-shard",
            "/deduct-with-shard-id/",
            "/deduct-db-only/",
            "/preheat-stock",
            "/internal/sync-stock",
            "/async-deduct-with-shard-id/",
            "/admin/grant",
            "/compensation"
    );

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        String path = exchange.getRequest().getURI().getPath();
        if (isBlocked(path)) {
            log.warn("拒绝经网关访问内部接口: path={}", path);
            exchange.getResponse().setStatusCode(HttpStatus.FORBIDDEN);
            exchange.getResponse().getHeaders().setContentType(MediaType.APPLICATION_JSON);
            byte[] body = "{\"code\":403,\"message\":\"forbidden: internal api\"}".getBytes(StandardCharsets.UTF_8);
            return exchange.getResponse().writeWith(Mono.just(exchange.getResponse().bufferFactory().wrap(body)));
        }
        return chain.filter(exchange);
    }

    private boolean isBlocked(String path) {
        if (path == null) {
            return false;
        }
        for (String prefix : BLOCKED_PREFIXES) {
            if (path.startsWith(prefix)) {
                return true;
            }
        }
        if (path.startsWith("/api/v1/coupon/")) {
            for (String marker : BLOCKED_CONTAINS) {
                if (path.contains(marker)) {
                    return true;
                }
            }
        }
        return false;
    }

    @Override
    public int getOrder() {
        return -200; // 早于 JWT 过滤器
    }
}
