package com.aliyun.seckill.couponkillconnectorservice.domain;

import com.aliyun.seckill.common.connector.PlatformType;
import com.fasterxml.jackson.annotation.JsonFormat;
import lombok.Data;

import java.math.BigDecimal;
import java.time.LocalDateTime;

/**
 * 同品比价手工映射（多平台参考 SKU / 手工价；不参与库存同步）。
 */
@Data
public class CouponPriceMap {
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long id;
    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Long couponId;
    private PlatformType platform;
    private String externalSkuId;
    private String title;
    private BigDecimal manualPrice;
    private String currency;
    private Boolean enabled;
    private LocalDateTime createTime;
    private LocalDateTime updateTime;
}
