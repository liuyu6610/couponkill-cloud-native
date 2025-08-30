// MySQLConfig.java
// 文件路径: com/aliyun/seckill/common/config/MySQLConfig.java
package com.aliyun.seckill.common.config;

import org.springframework.context.annotation.Configuration;

@Configuration
public class MySQLConfig {
    // 由于使用ShardingSphere进行数据源管理，移除原有的数据源配置
    // ShardingSphere配置由各服务的*ShardingSphereConfig类处理
}