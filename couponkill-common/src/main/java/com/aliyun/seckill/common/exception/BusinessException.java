// com.aliyun.seckill.common.exception.BusinessException.java
package com.aliyun.seckill.common.exception;

import com.aliyun.seckill.common.enums.ResultCode;
import lombok.Getter;

@Getter
public class BusinessException extends RuntimeException {
    private final int code;
    private final String message;

    public BusinessException(ResultCode resultCode) {
        this.code = resultCode.getCode();
        this.message = resultCode.getMessage();
    }

    public BusinessException(int code, String message) {
        this.code = code;
        this.message = message;
    }
}