package com.aliyun.seckill.common.context;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.context.request.RequestContextHolder;
import org.springframework.web.context.request.ServletRequestAttributes;

import jakarta.servlet.http.HttpServletRequest;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

/**
 * 请求用户上下文。JDK25：优先读 {@link ScopedValue}（由 {@code UserContextFilter} 绑定），
 * 回退到 Request 头，避免高并发 VT 下 ThreadLocal 膨胀。
 */
@Slf4j
public final class UserContext {

    public static final String USER_ID_HEADER = "X-User-Id";
    public static final String USER_ROLES_HEADER = "X-User-Roles";
    public static final String AUTHENTICATED_HEADER = "X-Authenticated";

    /** JDK 25 正式 Scoped Values（JEP 506） */
    public static final ScopedValue<String> SCOPED_USER_ID = ScopedValue.newInstance();
    public static final ScopedValue<String> SCOPED_USER_ROLES = ScopedValue.newInstance();
    public static final ScopedValue<Boolean> SCOPED_AUTHENTICATED = ScopedValue.newInstance();

    private UserContext() {
    }

    public static String getCurrentUserId() {
        if (SCOPED_USER_ID.isBound()) {
            String scoped = SCOPED_USER_ID.get();
            if (scoped != null && !scoped.isEmpty()) {
                return scoped;
            }
        }
        try {
            ServletRequestAttributes attributes =
                    (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
            if (attributes != null) {
                HttpServletRequest request = attributes.getRequest();
                String userId = request.getHeader(USER_ID_HEADER);
                if (userId == null || userId.isEmpty()) {
                    userId = request.getHeader("X-User-ID");
                }
                return userId;
            }
        } catch (Exception e) {
            log.warn("Failed to get current user id", e);
        }
        return null;
    }

    public static Long requireCurrentUserId() {
        String userId = getCurrentUserId();
        if (userId == null || userId.isBlank()) {
            throw new BusinessException(ResultCode.TOKEN_INVALID.getCode(), "未登录或身份无效");
        }
        try {
            return Long.parseLong(userId.trim());
        } catch (NumberFormatException e) {
            throw new BusinessException(ResultCode.AUTH_FAIL.getCode(), "用户身份格式错误");
        }
    }

    public static List<String> getCurrentUserRoles() {
        if (SCOPED_USER_ROLES.isBound()) {
            String roles = SCOPED_USER_ROLES.get();
            if (roles != null && !roles.isEmpty()) {
                return Arrays.asList(roles.split(","));
            }
        }
        try {
            ServletRequestAttributes attributes =
                    (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
            if (attributes != null) {
                HttpServletRequest request = attributes.getRequest();
                String roles = request.getHeader(USER_ROLES_HEADER);
                if (roles != null && !roles.isEmpty()) {
                    return Arrays.asList(roles.split(","));
                }
            }
        } catch (Exception e) {
            log.warn("Failed to get current user roles", e);
        }
        return Collections.emptyList();
    }

    public static boolean isAuthenticated() {
        if (SCOPED_AUTHENTICATED.isBound()) {
            Boolean auth = SCOPED_AUTHENTICATED.get();
            if (auth != null) {
                return auth;
            }
        }
        try {
            ServletRequestAttributes attributes =
                    (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
            if (attributes != null) {
                HttpServletRequest request = attributes.getRequest();
                return "true".equals(request.getHeader(AUTHENTICATED_HEADER));
            }
        } catch (Exception e) {
            log.warn("Failed to check authentication status", e);
        }
        return false;
    }

    public static boolean hasRole(String role) {
        return getCurrentUserRoles().contains(role);
    }

    public static boolean hasAnyRole(String... roles) {
        List<String> userRoles = getCurrentUserRoles();
        for (String role : roles) {
            if (userRoles.contains(role)) {
                return true;
            }
        }
        return false;
    }
}
