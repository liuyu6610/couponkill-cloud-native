package com.aliyun.seckill.common.api;
import lombok.*;

@Getter @Setter @AllArgsConstructor @NoArgsConstructor @Builder
public class ApiResp<T> {
    private int code;        // 0=OK, 非0=错误
    private String message;  // 文案
    private String requestId;// 关联ID，可为空
    private String traceId;  // 可由网关/链路注入
    private T data;

    public static <T> ApiResp<T> ok(T data, String reqId){ return ApiResp.<T>builder().code(0).message("OK").requestId(reqId).data(data).build();}
    public static <T> ApiResp<T> err(int code, String msg, String reqId){ return ApiResp.<T>builder().code(code).message(msg).requestId(reqId).build();}
}
