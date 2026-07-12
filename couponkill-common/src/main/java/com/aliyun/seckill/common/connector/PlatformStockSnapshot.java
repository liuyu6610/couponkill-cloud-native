package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 平台库存快照。
 * stockQty 为可写入本仓 Redis 的数量；若平台仅返回「有货/无货」而无精确数量，
 * 可用 stockAvailable + stockQty=null 表达，由同步策略决定如何映射。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PlatformStockSnapshot {
    private PlatformType platform;
    private String externalSkuId;
    /** 精确库存；null 表示平台未给出精确数 */
    private Long stockQty;
    /** 是否有货（状态语义） */
    private Boolean stockAvailable;
    private String stockStateDesc;
    private String area;
}
