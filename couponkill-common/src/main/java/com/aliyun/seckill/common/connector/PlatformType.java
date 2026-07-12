package com.aliyun.seckill.common.connector;

/**
 * 电商平台类型（Connector SPI）。
 * TB / PDD 为二期预留；本期仅 stub，health=DISABLED。
 */
public enum PlatformType {
    MOCK,
    JD,
    /** 淘宝开放平台（二期） */
    TB,
    /** 拼多多开放平台（二期） */
    PDD
}
