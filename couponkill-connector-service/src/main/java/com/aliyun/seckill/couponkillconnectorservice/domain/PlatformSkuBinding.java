package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.aliyun.seckill.common.connector.PlatformType;
import lombok.Data;

import java.time.LocalDateTime;

@Data
public class PlatformSkuBinding {
    private Long id;
    private PlatformType platform;
    private String externalSkuId;
    private Long couponId;
    private Boolean syncEnabled;
    private Long lastStock;
    private LocalDateTime lastSyncAt;
    private String lastSyncStatus;
    private String lastError;
    private LocalDateTime createTime;
    private LocalDateTime updateTime;
}
