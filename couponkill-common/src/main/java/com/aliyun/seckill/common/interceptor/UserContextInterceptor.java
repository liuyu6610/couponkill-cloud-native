// 文件路径: com/aliyun/seckill/common/interceptor/UserContextInterceptor.java
package com.aliyun.seckill.common.interceptor;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import org.springframework.web.servlet.HandlerInterceptor;

@Slf4j
@Component
public class UserContextInterceptor implements HandlerInterceptor {

    @Override
    public boolean preHandle(HttpServletRequest request, HttpServletResponse response, Object handler) {
        // 可以在这里添加一些预处理逻辑
        log.debug("Processing request with UserContextInterceptor");
        return true;
    }
}
