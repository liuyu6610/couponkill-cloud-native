package com.aliyun.seckill.common.connector;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Connector 健康状态。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ConnectorHealth {
    private PlatformType platform;
    /** UP / DOWN / DISABLED */
    private String status;
    private String message;
}
