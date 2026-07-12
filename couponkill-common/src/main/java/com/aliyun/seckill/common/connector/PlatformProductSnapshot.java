package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.math.BigDecimal;

/**
 * 平台商品快照（只读）。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PlatformProductSnapshot {
    private PlatformType platform;
    private String externalSkuId;
    private String title;
    private BigDecimal price;
    /** 是否可售/上架 */
    private Boolean onSale;
    private String rawStatus;
}
