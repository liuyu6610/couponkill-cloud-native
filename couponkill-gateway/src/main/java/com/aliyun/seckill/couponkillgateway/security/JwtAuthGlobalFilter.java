package com.aliyun.seckill.couponkillgateway.security;

import com.aliyun.seckill.couponkillgateway.utils.JwtUtil;
import io.jsonwebtoken.Claims;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cloud.gateway.filter.GatewayFilterChain;
import org.springframework.cloud.gateway.filter.GlobalFilter;
import org.springframework.core.Ordered;
import org.springframework.http.HttpStatus;
import org.springframework.http.server.reactive.ServerHttpRequest;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;

import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

@Slf4j
@Component
public class JwtAuthGlobalFilter implements GlobalFilter, Ordered {

    private static final List<String> WHITE_LIST = List.of(
            "/fallback/",
            "/api/v1/user/register",
            "/api/v1/user/login",
            "/api/v1/auth/token/mock",
            // Connector 探活：仅健康检查免登录，绑定/同步仍需 JWT+admin
            "/api/v1/connector/health"
    );

    @Autowired
    private JwtUtil jwtUtil;

    /** 逗号分隔的管理员 userId，本地 demo=10000 */
    @Value("${connector.admin.user-ids:10000}")
    private String adminUserIds;

    /** 可选：管理头令牌，与 JWT admin 角色二选一 */
    @Value("${connector.admin.token:}")
    private String adminToken;

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        ServerHttpRequest request = exchange.getRequest();
        String path = request.getURI().getPath();

        log.debug("收到请求: path={}", path);

        if (isWhitelisted(path)) {
            log.debug("路径在白名单中，跳过认证: path={}", path);
            return chain.filter(exchange);
        }

        // Connector 管理面：JWT + admin（或 X-Admin-Token）
        if (isConnectorAdminPath(path)) {
            String headerAdmin = request.getHeaders().getFirst("X-Admin-Token");
            if (StringUtils.hasText(adminToken) && adminToken.equals(headerAdmin)) {
                ServerHttpRequest mutated = request.mutate()
                        .header("X-Authenticated", "true")
                        .header("X-User-Roles", "admin")
                        .build();
                return chain.filter(exchange.mutate().request(mutated).build());
            }
        }

        String token = extractToken(request);
        if (token == null || token.isEmpty()) {
            log.warn("请求缺少认证token: path={}", path);
            exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
            return exchange.getResponse().setComplete();
        }

        try {
            if (!jwtUtil.verifyToken(token)) {
                log.warn("Token验证失败: path={}", path);
                exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
                return exchange.getResponse().setComplete();
            }

            String userId = jwtUtil.getUserId(token);
            if (userId == null || userId.isEmpty()) {
                log.warn("Token中userId为空: path={}", path);
                exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
                return exchange.getResponse().setComplete();
            }

            Claims claims = jwtUtil.parse(token);
            List<String> roles = extractRoles(claims);
            boolean admin = isAdmin(userId, roles);

            if (isConnectorAdminPath(path) && !admin) {
                log.warn("拒绝非管理员访问 Connector: userId={}, path={}", userId, path);
                exchange.getResponse().setStatusCode(HttpStatus.FORBIDDEN);
                return exchange.getResponse().setComplete();
            }

            ServerHttpRequest mutatedRequest = request.mutate()
                    .header("X-User-Id", userId)
                    .header("X-Authenticated", "true")
                    .header("X-User-Roles", String.join(",", roles))
                    .build();
            return chain.filter(exchange.mutate().request(mutatedRequest).build());
        } catch (Exception e) {
            log.error("认证过程出现异常: path={}, error={}", path, e.getMessage(), e);
            exchange.getResponse().setStatusCode(HttpStatus.UNAUTHORIZED);
            return exchange.getResponse().setComplete();
        }
    }

    private boolean isConnectorAdminPath(String path) {
        return path != null
                && path.startsWith("/api/v1/connector/")
                && !path.equals("/api/v1/connector/health");
    }

    private boolean isAdmin(String userId, List<String> roles) {
        if (roles != null && roles.stream().anyMatch(r -> "admin".equalsIgnoreCase(r))) {
            return true;
        }
        Set<String> allow = parseAdminIds();
        return allow.contains(userId);
    }

    private Set<String> parseAdminIds() {
        if (!StringUtils.hasText(adminUserIds)) {
            return Collections.emptySet();
        }
        return Arrays.stream(adminUserIds.split(","))
                .map(String::trim)
                .filter(s -> !s.isEmpty())
                .collect(Collectors.toCollection(HashSet::new));
    }

    @SuppressWarnings("unchecked")
    private List<String> extractRoles(Claims claims) {
        Object roles = claims.get("roles");
        if (roles instanceof List<?> list) {
            return list.stream().map(String::valueOf).toList();
        }
        return List.of("user");
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
        return -100;
    }
}
