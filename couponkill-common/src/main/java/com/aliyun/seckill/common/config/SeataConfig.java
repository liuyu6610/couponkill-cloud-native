// couponkill-common/src/main/java/com/aliyun/seckill/common/config/SeataConfig.java
package com.aliyun.seckill.common.config;

import io.seata.spring.annotation.GlobalTransactionScanner;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SeataConfig {

    @Value("${spring.application.name:unknown-service}")
    private String applicationName;

    @Value("${seata.tx-service-group:default_tx_group}")
    private String txServiceGroup;

    @Bean
    public GlobalTransactionScanner globalTransactionScanner() {
        return new GlobalTransactionScanner(applicationName, txServiceGroup);
    }
}
