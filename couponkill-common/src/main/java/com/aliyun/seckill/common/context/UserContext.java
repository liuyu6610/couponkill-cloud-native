// 文件路径: com/aliyun/seckill/common/context/UserContext.java
package com.aliyun.seckill.common.context;

import lombok.extern.slf4j.Slf4j;
import org.springframework.web.context.request.RequestContextHolder;
import org.springframework.web.context.request.ServletRequestAttributes;

import jakarta.servlet.http.HttpServletRequest;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

@Slf4j
public class UserContext {

    public static final String USER_ID_HEADER = "X-User-Id";
    public static final String USER_ROLES_HEADER = "X-User-Roles";
    public static final String AUTHENTICATED_HEADER = "X-Authenticated";

    /**
     * 获取当前用户ID
     * @return 用户ID，如果未认证则返回null
     */
    public static String getCurrentUserId() {
        try {
            ServletRequestAttributes attributes = (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
            if (attributes != null) {
                HttpServletRequest request = attributes.getRequest();
                return request.getHeader(USER_ID_HEADER);
            }
        } catch (Exception e) {
            log.warn("Failed to get current user id", e);
        }
        return null;
    }

    /**
     * 获取当前用户角色
     * @return 用户角色列表
     */
    public static List<String> getCurrentUserRoles() {
        try {
            ServletRequestAttributes attributes = (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
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

    /**
     * 检查是否已认证
     * @return 是否已认证
     */
    public static boolean isAuthenticated() {
        try {
            ServletRequestAttributes attributes = (ServletRequestAttributes) RequestContextHolder.getRequestAttributes();
            if (attributes != null) {
                HttpServletRequest request = attributes.getRequest();
                String authenticated = request.getHeader(AUTHENTICATED_HEADER);
                return "true".equals(authenticated);
            }
        } catch (Exception e) {
            log.warn("Failed to check authentication status", e);
        }
        return false;
    }

    /**
     * 检查用户是否有指定角色
     * @param role 角色
     * @return 是否有指定角色
     */
    public static boolean hasRole(String role) {
        return getCurrentUserRoles().contains(role);
    }

    /**
     * 检查用户是否有任意一个指定角色
     * @param roles 角色列表
     * @return 是否有任意一个指定角色
     */
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
