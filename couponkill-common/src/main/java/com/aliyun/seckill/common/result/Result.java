// com.aliyun.seckill.common.result.Result.java
package com.aliyun.seckill.common.result;

import com.aliyun.seckill.common.enums.ResultCode;
import lombok.Data;

import java.util.Map;

@Data
public class Result<T> {
    private int code;
    private String message;
    private T data;

    private Result() {}

    public static <T> Result<T> success() {
        Result<T> result = new Result<>();
        result.code = ResultCode.SUCCESS.getCode();
        result.message = ResultCode.SUCCESS.getMessage();
        return result;
    }

    public static <T> Result<T> success(T data) {
        Result<T> result = new Result<>();
        result.code = ResultCode.SUCCESS.getCode();
        result.message = ResultCode.SUCCESS.getMessage();
        result.data = data;
        return result;
    }

    public static <T> Result<T> fail(ResultCode resultCode) {
        Result<T> result = new Result<>();
        result.code = resultCode.getCode();
        result.message = resultCode.getMessage();
        return result;
    }

    public static <T> Result<T> fail(int code, String message) {
        Result<T> result = new Result<>();
        result.code = code;
        result.message = message;
        return result;
    }

   public static Result<Map<String, Object>> error(String message) {
    Result<Map<String, Object>> result = new Result<>();
    result.code = ResultCode.FAIL.getCode();
    result.message = message;
    return result;
}

}