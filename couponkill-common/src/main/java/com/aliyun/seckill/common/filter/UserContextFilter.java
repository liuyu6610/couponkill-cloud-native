package com.aliyun.seckill.common.filter;

import com.aliyun.seckill.common.context.UserContext;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.boot.autoconfigure.condition.ConditionalOnWebApplication;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;

/**
 * 将网关注入的用户头绑定到 JDK25 ScopedValue，整条请求链路（含 VT 子任务）可继承。
 */
@Component
@ConditionalOnWebApplication(type = ConditionalOnWebApplication.Type.SERVLET)
@Order(Ordered.HIGHEST_PRECEDENCE + 20)
public class UserContextFilter extends OncePerRequestFilter {

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response, FilterChain filterChain)
            throws ServletException, IOException {
        String userId = request.getHeader(UserContext.USER_ID_HEADER);
        if (userId == null || userId.isEmpty()) {
            userId = request.getHeader("X-User-ID");
        }
        String roles = request.getHeader(UserContext.USER_ROLES_HEADER);
        boolean authenticated = "true".equals(request.getHeader(UserContext.AUTHENTICATED_HEADER))
                || (userId != null && !userId.isBlank());

        String uid = userId == null ? "" : userId;
        String roleStr = roles == null ? "" : roles;
        try {
            ScopedValue.where(UserContext.SCOPED_USER_ID, uid)
                    .where(UserContext.SCOPED_USER_ROLES, roleStr)
                    .where(UserContext.SCOPED_AUTHENTICATED, authenticated)
                    .run(() -> {
                        try {
                            filterChain.doFilter(request, response);
                        } catch (IOException | ServletException e) {
                            throw new FilterException(e);
                        }
                    });
        } catch (FilterException e) {
            Throwable cause = e.getCause();
            if (cause instanceof IOException io) {
                throw io;
            }
            if (cause instanceof ServletException se) {
                throw se;
            }
            throw e;
        }
    }

    private static final class FilterException extends RuntimeException {
        FilterException(Throwable cause) {
            super(cause);
        }
    }
}
