package com.aliyun.seckill.couponkillorderservice.domain;

import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;

import java.time.LocalDateTime;

@Data
public class UserNotification {
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long id;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long userId;
    /** RESERVATION_SUCCESS / RESERVATION_FAILED / RESERVATION_EXPIRED */
    private String type;
    private String title;
    private String content;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long refId;
    private Boolean readFlag;
    private LocalDateTime createTime;
}
