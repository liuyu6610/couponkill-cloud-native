// couponkill-common/src/main/java/com/aliyun/seckill/common/mq/RocketMQConfig.java
package com.aliyun.seckill.common.config;

import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class RocketMQBaseConfig {

    @Bean
    public RocketMQTemplate rocketMQTemplate() {
        return new RocketMQTemplate();
    }
}