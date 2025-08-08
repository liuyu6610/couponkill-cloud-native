// couponkill-common/src/main/java/com/aliyun/seckill/common/config/SeataConfig.java
package com.aliyun.seckill.common.config;

import io.seata.spring.annotation.GlobalTransactionScanner;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SeataConfig {

    @Value("${spring.application.name}")
    private String applicationName;

    @Value("${seata.tx-service-group}")
    private String txServiceGroup;

    @Bean
    public GlobalTransactionScanner globalTransactionScanner() {
        // 使用配置文件中的应用名和事务组，而非硬编码
        return new GlobalTransactionScanner(applicationName, txServiceGroup);
    }
}