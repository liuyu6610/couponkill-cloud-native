// com.aliyun.seckill.common.exception.GlobalExceptionHandler.java
package com.aliyun.seckill.common.exception;

import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.result.ResultCode;
import lombok.extern.slf4j.Slf4j;
import org.springframework.validation.BindingResult;
import org.springframework.validation.FieldError;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

@Slf4j
@RestControllerAdvice
public class GlobalExceptionHandler {

    @ExceptionHandler(BusinessException.class)
    public Result<?> handleBusinessException(BusinessException e) {
        log.error("业务异常: {}", e.getMessage(), e);
        return Result.error(e.getCode(), e.getMessage());
    }

    @ExceptionHandler(MethodArgumentNotValidException.class)
    public Result<?> handleMethodArgumentNotValidException(MethodArgumentNotValidException e) {
        BindingResult bindingResult = e.getBindingResult();
        StringBuilder errorMsg = new StringBuilder();
        for (FieldError fieldError : bindingResult.getFieldErrors()) {
            errorMsg.append(fieldError.getField()).append(":").append(fieldError.getDefaultMessage()).append(",");
        }
        String msg = errorMsg.toString();
        if (msg.endsWith(",")) {
            msg = msg.substring(0, msg.length() - 1);
        }
        log.error("参数校验异常: {}", msg);
        return Result.error(ResultCode.PARAM_ERROR.getCode(), msg);
    }

    @ExceptionHandler(Exception.class)
    public Result<?> handleException(Exception e) {
        log.error("系统异常: {}", e.getMessage(), e);
        return Result.error(ResultCode.SYSTEM_ERROR);
    }
}