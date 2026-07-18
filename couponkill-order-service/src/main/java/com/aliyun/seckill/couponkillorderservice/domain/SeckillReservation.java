package com.aliyun.seckill.couponkillorderservice.domain;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;

import java.time.LocalDateTime;

/**
 * 秒杀预约帮抢记录。
 * 状态机：PENDING → FIRING → QUEUED → SUCCESS / FAILED；
 * PENDING 亦可 → CANCELLED / EXPIRED。
 */
@Data
public class SeckillReservation {
    /** 跨 JS 边界以字符串传输，避免大整数精度丢失 */
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long id;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long userId;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long couponId;
    private String status;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime reserveAt;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime triggerAt;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime firedAt;
    private String requestId;
    private String orderId;
    private Integer failCode;
    private String failReason;
    private Integer retryCount;
    private Integer version;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime createTime;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime updateTime;
}
