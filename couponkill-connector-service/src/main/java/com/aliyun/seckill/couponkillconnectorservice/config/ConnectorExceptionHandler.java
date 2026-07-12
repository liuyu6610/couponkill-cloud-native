package com.aliyun.seckill.couponkillconnectorservice.config;

import com.aliyun.seckill.common.api.ApiResponse;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

/**
 * Connector 服务本地异常映射（未扫描 common GlobalExceptionHandler）。
 */
@Slf4j
@RestControllerAdvice
public class ConnectorExceptionHandler {

    @ExceptionHandler({IllegalArgumentException.class, IllegalStateException.class, UnsupportedOperationException.class})
    public ApiResponse<Void> handleClientError(RuntimeException e) {
        log.warn("connector client error: {}", e.getMessage());
        return ApiResponse.fail(400, e.getMessage());
    }

    @ExceptionHandler(Exception.class)
    public ApiResponse<Void> handleUnexpected(Exception e) {
        log.error("connector unexpected error", e);
        return ApiResponse.fail(500, "系统异常: " + e.getMessage());
    }
}
