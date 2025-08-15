package com.aliyun.seckill.common.exception;

import com.aliyun.seckill.common.api.ApiResponse;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

@Slf4j
@RestControllerAdvice
public class GlobalExceptionHandler {

    @ExceptionHandler(BusinessException.class)
    public ApiResponse<String> handleBusiness(BusinessException e) {
        log.warn("business error", e);
        return ApiResponse.fail(e.getCode(), e.getMessage());
    }

    @ExceptionHandler(Exception.class)
    public ApiResponse<String> handleException(Exception e) {
        log.error("unexpected error", e);
        return ApiResponse.fail(500, "系统异常: " + e.getMessage());
    }
}
