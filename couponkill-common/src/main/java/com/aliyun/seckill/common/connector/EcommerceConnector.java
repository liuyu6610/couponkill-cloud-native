package com.aliyun.seckill.common.connector;

/**
 * 电商平台适配 SPI。实现类不得出现在秒杀热路径（Lua/Kafka 入队）中。
 */
public interface EcommerceConnector {

    PlatformType platform();

    PlatformProductSnapshot getProduct(String externalSkuId);

    PlatformStockSnapshot getStock(String externalSkuId);

    ConnectorHealth health();
}
