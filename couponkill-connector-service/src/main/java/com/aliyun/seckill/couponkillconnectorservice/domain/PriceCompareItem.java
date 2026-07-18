package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.aliyun.seckill.common.connector.PlatformType;
import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.math.BigDecimal;
import java.time.LocalDateTime;

/**
 * 同品比价条目（对齐 PRODUCT P1-1）。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PriceCompareItem {
    private PlatformType platform;
    private String externalSkuId;
    private String title;
    private BigDecimal price;
    private String currency;
    @JsonFormat(pattern = "yyyy-MM-dd HH:mm:ss", timezone = "GMT+8")
    private LocalDateTime fetchedAt;
    /** PROBE / MANUAL / BINDING */
    private String source;
    /** HIGH / MEDIUM / LOW */
    private String confidence;
    private String message;
}
