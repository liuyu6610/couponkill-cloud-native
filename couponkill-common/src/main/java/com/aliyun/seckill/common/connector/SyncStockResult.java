package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * coupon-service 同步 Redis 库存结果：appliedStock 为实际落库值（安全合并后可能低于 target）。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class SyncStockResult {
    private boolean success;
    /** 平台/调用方请求的目标库存 */
    private Long targetStock;
    /** Redis 实际库存（安全合并后可能保持原值） */
    private Long appliedStock;
    /** 是否发生了写入（含下调/强制覆盖/首次写入） */
    private Boolean changed;
    private String message;
}
