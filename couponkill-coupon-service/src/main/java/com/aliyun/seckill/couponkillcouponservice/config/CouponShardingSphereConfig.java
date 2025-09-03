package com.aliyun.seckill.couponkillcouponservice.config;

import com.alibaba.nacos.api.NacosFactory;
import com.alibaba.nacos.api.config.ConfigService;
import com.alibaba.nacos.api.config.listener.Listener;
import com.alibaba.nacos.api.exception.NacosException;
import lombok.extern.slf4j.Slf4j;
import org.apache.shardingsphere.driver.api.yaml.YamlShardingSphereDataSourceFactory;
import org.apache.shardingsphere.driver.jdbc.core.datasource.ShardingSphereDataSource;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

import javax.sql.DataSource;
import java.io.IOException;
import java.sql.SQLException;
import java.util.Properties;
import java.util.concurrent.Executor;
import java.util.concurrent.Executors;

@Slf4j
@Configuration
public class CouponShardingSphereConfig {

    @Value("${spring.cloud.nacos.config.server-addr:localhost:8848}")
    private String nacosServerAddr;

    @Value("${shardingsphere.namespace:shardingsphere-namespace-id}")
    private String namespace;

    @Value("${spring.cloud.nacos.config.group:DEFAULT_GROUP}")
    private String group;

    private ConfigService configService;
    private DataSource dataSource;

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

            configService = NacosFactory.createConfigService(properties);

            // 从Nacos获取coupon-service的ShardingSphere配置
            String dataId = "coupon-service-sharding.yaml";
            String configContent = configService.getConfig(dataId, group, 3000);

            if (configContent == null || configContent.isEmpty()) {
                log.warn("从Nacos获取的配置为空，将使用默认配置");
                // 这里应该加载默认配置
                throw new RuntimeException("无法从Nacos获取分库分表配置");
            }

            log.info("从Nacos获取到的ShardingSphere配置: {}", configContent);

            // 创建ShardingSphere数据源
            dataSource = YamlShardingSphereDataSourceFactory.createDataSource(configContent.getBytes());

            // 添加配置监听器，实现配置的动态更新
            configService.addListener(dataId, group, new Listener() {
                @Override
                public Executor getExecutor() {
                    return Executors.newSingleThreadExecutor();
                }

                @Override
                public void receiveConfigInfo(String configInfo) {
                    log.info("接收到Nacos配置变更通知，新的配置内容: {}", configInfo);
                    try {
                        // 创建新的数据源
                        DataSource newDataSource = YamlShardingSphereDataSourceFactory.createDataSource(configInfo.getBytes());
                        
                        // 关闭旧的数据源
                        if (dataSource != null) {
                            try {
                                // 安全关闭ShardingSphere数据源
                                if (dataSource instanceof ShardingSphereDataSource) {
                                    ((ShardingSphereDataSource) dataSource).close();
                                }
                            } catch (Exception e) {
                                log.warn("关闭旧数据源时发生异常: ", e);
                            }
                        }
                        
                        // 替换数据源
                        dataSource = newDataSource;
                        log.info("ShardingSphere数据源已更新");
                    } catch (Exception e) {
                        log.error("更新ShardingSphere数据源失败: ", e);
                    }
                }
            });

            return dataSource;
        } catch (NacosException e) {
            log.error("连接Nacos失败", e);
            throw e;
        } catch (Exception e) {
            log.error("创建ShardingSphere数据源失败", e);
            throw new RuntimeException("创建ShardingSphere数据源失败", e);
        }
    }

    /**
     * 获取当前数据源（用于配置更新）
     * @return 当前数据源
     */
    public DataSource getCurrentDataSource() {
        return dataSource;
    }

    /**
     * 重新加载数据源配置
     * @throws SQLException SQL异常
     * @throws IOException IO异常
     * @throws NacosException Nacos异常
     */
    public void reloadDataSource() throws SQLException, IOException, NacosException {
        // 从Nacos重新获取配置
        String dataId = "coupon-service-sharding.yaml";
        String configContent = configService.getConfig(dataId, group, 3000);
        
        if (configContent != null && !configContent.isEmpty()) {
            log.info("重新加载ShardingSphere配置: {}", configContent);
            
            // 创建新的数据源
            DataSource newDataSource = YamlShardingSphereDataSourceFactory.createDataSource(configContent.getBytes());
            
            // 关闭旧的数据源
            if (dataSource != null) {
                try {
                    // 安全关闭ShardingSphere数据源
                    if (dataSource instanceof ShardingSphereDataSource) {
                        ((ShardingSphereDataSource) dataSource).close();
                    }
                } catch (Exception e) {
                    log.warn("关闭旧数据源时发生异常: ", e);
                }
            }
            
            // 替换数据源
            dataSource = newDataSource;
            log.info("ShardingSphere数据源已重新加载");
        } else {
            log.warn("从Nacos获取的配置为空，未重新加载数据源");
        }
    }
}