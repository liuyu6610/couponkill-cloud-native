// couponkill-common/src/main/java/com/aliyun/seckill/common/config/SeataConfig.java
package com.aliyun.seckill.common.config;

import io.seata.spring.annotation.GlobalTransactionScanner;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class SeataConfig {

    @Bean
    public GlobalTransactionScanner globalTransactionScanner() {
        String applicationName = "couponkill-common";
        String txServiceGroup = "couponkill-group";
        return new GlobalTransactionScanner(applicationName, txServiceGroup);
    }
}