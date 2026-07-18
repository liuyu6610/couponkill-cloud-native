package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.aliyun.seckill.common.connector.PlatformType;
import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;

import java.time.LocalDateTime;

@Data
public class PlatformSkuBinding {
    /** 跨 JS 边界以字符串传输，避免大整数精度丢失 */
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long id;
    private PlatformType platform;
    private String externalSkuId;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long couponId;
    private Boolean syncEnabled;
    private Long lastStock;
    private LocalDateTime lastSyncAt;
    private String lastSyncStatus;
    private String lastError;
    private LocalDateTime createTime;
    private LocalDateTime updateTime;
}
