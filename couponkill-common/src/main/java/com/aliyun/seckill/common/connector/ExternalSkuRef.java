package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 外部平台 SKU 引用。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ExternalSkuRef {
    private PlatformType platform;
    private String externalSkuId;
}
