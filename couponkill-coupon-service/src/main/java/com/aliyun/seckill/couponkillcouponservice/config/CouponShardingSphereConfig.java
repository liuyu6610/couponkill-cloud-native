package com.aliyun.seckill.couponkillcouponservice.config;

import com.alibaba.nacos.api.NacosFactory;
import com.alibaba.nacos.api.config.ConfigService;
import com.alibaba.nacos.api.exception.NacosException;
import org.apache.shardingsphere.driver.api.yaml.YamlShardingSphereDataSourceFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

import javax.sql.DataSource;
import java.io.IOException;
import java.sql.SQLException;
import java.util.Properties;

@Configuration
public class CouponShardingSphereConfig {

    @Value("${spring.cloud.nacos.config.server-addr:localhost:8848}")
    private String nacosServerAddr;

    @Value("${shardingsphere.namespace:shardingsphere-namespace-id}")
    private String namespace;

    @Value("${spring.cloud.nacos.config.group:DEFAULT_GROUP}")
    private String group;

    @Bean
    @Primary
    public DataSource dataSource() throws SQLException, IOException, NacosException {
        try {
            // 构建Nacos配置服务
            Properties properties = new Properties();
            properties.put("serverAddr", nacosServerAddr);
            if (namespace != null && !namespace.isEmpty()) {
                properties.put("namespace", namespace);
            }

            ConfigService configService = NacosFactory.createConfigService(properties);

            // 从Nacos获取coupon-service的ShardingSphere配置
            String dataId = "coupon-service-sharding.yaml";
            String configContent = configService.getConfig(dataId, group, 3000);

            if (configContent == null || configContent.isEmpty()) {
                throw new IllegalStateException("未能从Nacos获取coupon-service的ShardingSphere配置，dataId: " + dataId);
            }

            // 使用从Nacos获取的配置创建数据源（不使用Seata代理）
            return YamlShardingSphereDataSourceFactory.createDataSource(configContent.getBytes());
        } catch (Exception e) {
            // 如果Nacos配置获取失败，使用默认配置
            throw new RuntimeException("数据源配置失败: " + e.getMessage(), e);
        }
    }
}
