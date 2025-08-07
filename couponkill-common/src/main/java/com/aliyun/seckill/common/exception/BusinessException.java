// com.aliyun.seckill.common.exception.BusinessException.java
package com.aliyun.seckill.common.exception;

import lombok.Getter;

@Getter
public class BusinessException extends RuntimeException {
    private int code;

    public BusinessException(int code, String message) {
        super(message);
        this.code = code;
    }

    public BusinessException(String message) {
        super(message);
        this.code = 500;
    }

    public BusinessException(Throwable cause) {
        super(cause);
        this.code = 500;
    }
}