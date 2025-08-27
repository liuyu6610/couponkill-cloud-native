package com.aliyun.seckill.common.utils;

import lombok.extern.slf4j.Slf4j;

@Slf4j
public class ShardingUtils {

    /**
     * 根据虚拟ID计算数据库索引
     * @param virtualId 虚拟ID
     * @param dbCount 数据库数量
     * @return 数据库索引
     */
    public static int getDatabaseIndex(String virtualId, int dbCount) {
        try {
            String[] parts = virtualId.split("_");
            if (parts.length >= 2) {
                int suffix = Integer.parseInt(parts[parts.length - 1]);
                return Math.abs(suffix) % dbCount;
            }
        } catch (Exception e) {
            log.warn("解析虚拟ID获取数据库索引失败，virtualId: {}", virtualId, e);
        }
        return 0;
    }

    /**
     * 根据虚拟ID计算表索引
     * @param virtualId 虚拟ID
     * @param tableCount 表数量
     * @return 表索引
     */
    public static int getTableIndex(String virtualId, int tableCount) {
        try {
            String[] parts = virtualId.split("_");
            if (parts.length >= 2) {
                int suffix = Integer.parseInt(parts[parts.length - 1]);
                return Math.abs(suffix) % tableCount;
            }
        } catch (Exception e) {
            log.warn("解析虚拟ID获取表索引失败，virtualId: {}", virtualId, e);
        }
        return 0;
    }
}
