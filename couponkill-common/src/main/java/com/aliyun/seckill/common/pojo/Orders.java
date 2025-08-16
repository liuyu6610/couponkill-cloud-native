package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.time.LocalDateTime;

@Data
public class Orders {
    private Long id;
    private String couponId;
    private LocalDateTime createdAt;
    private String requestId;
    private String status;
    private String userId;
}
