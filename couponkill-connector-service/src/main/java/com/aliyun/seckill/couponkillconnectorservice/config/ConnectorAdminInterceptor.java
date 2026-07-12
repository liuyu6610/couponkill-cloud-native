package com.aliyun.seckill.couponkillconnectorservice.config;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;
import org.springframework.web.servlet.HandlerInterceptor;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/**
 * 防御纵深：即便绕过网关直连 connector，管理写接口仍需 admin 角色头或 X-Admin-Token。
 * 网关会注入 X-User-Roles；本地可用 CONNECTOR_ADMIN_TOKEN。
 */
@Slf4j
@Component
public class ConnectorAdminInterceptor implements HandlerInterceptor {

    @Value("${connector.admin.token:}")
    private String adminToken;

    @Value("${connector.admin.allow-unauthenticated-local:false}")
    private boolean allowUnauthLocal;

    @Override
    public boolean preHandle(HttpServletRequest request, HttpServletResponse response, Object handler)
            throws Exception {
        String path = request.getRequestURI();
        if (path == null) {
            return true;
        }
        if (path.equals("/api/v1/connector/health") || path.startsWith("/actuator")) {
            return true;
        }
        if (!path.startsWith("/api/v1/connector/")) {
            return true;
        }
        if (allowUnauthLocal) {
            log.warn("connector.admin.allow-unauthenticated-local=true，跳过管理鉴权（仅本地）");
            return true;
        }

        String headerToken = request.getHeader("X-Admin-Token");
        if (StringUtils.hasText(adminToken) && adminToken.equals(headerToken)) {
            return true;
        }
        String roles = request.getHeader("X-User-Roles");
        if (StringUtils.hasText(roles)
                && Arrays.stream(roles.split(",")).map(String::trim).anyMatch("admin"::equalsIgnoreCase)) {
            return true;
        }

        log.warn("拒绝非管理员访问 connector: path={}, roles={}", path, roles);
        response.setStatus(HttpServletResponse.SC_FORBIDDEN);
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        response.getOutputStream().write(
                "{\"code\":403,\"message\":\"forbidden: connector admin required\"}".getBytes(StandardCharsets.UTF_8));
        return false;
    }
}
