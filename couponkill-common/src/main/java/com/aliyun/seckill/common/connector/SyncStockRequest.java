package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 将平台库存同步到本仓 Redis 的内部请求（coupon-service）。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SyncStockRequest {
    private Long couponId;
    /** 目标 Redis 库存（coupon:stock:{couponId}） */
    private Long targetStock;
    /** 可选：来源平台，仅审计 */
    private PlatformType sourcePlatform;
    private String externalSkuId;
    /**
     * true=强制覆盖（校准窗口）；false/缺省=安全合并：永不抬高已有库存。
     */
    private Boolean force;
}
