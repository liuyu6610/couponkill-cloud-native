package com.aliyun.seckill.couponkillorderservice.config;

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

import javax.sql.DataSource;
import java.io.IOException;
import java.sql.SQLException;
import java.util.Properties;
import java.util.concurrent.Executor;

@Slf4j
@Configuration
public class OrderShardingSphereConfig {

    @Value("${spring.cloud.nacos.config.server-addr:localhost:8848}")
    private String nacosServerAddr;

    @Value("${shardingsphere.namespace:shardingsphere-namespace-id}")
    private String namespace;

    @Value("${spring.cloud.nacos.config.group:DEFAULT_GROUP}")
    private String group;

    private ConfigService configService;
    private DataSource dataSource;

    @Bean
    public DataSource dataSource() throws SQLException, IOException, NacosException {
        // 构建Nacos配置服务
        Properties properties = new Properties();
        properties.put("serverAddr", nacosServerAddr);
        if (namespace != null && !namespace.isEmpty()) {
            properties.put("namespace", namespace);
        }

        configService = NacosFactory.createConfigService(properties);

        // 从Nacos获取order-service的ShardingSphere配置
        String dataId = "order-service-sharding.yaml";
        String configContent = configService.getConfig(dataId, group, 3000);

        if (configContent == null || configContent.isEmpty()) {
            throw new IllegalStateException("未能从Nacos获取order-service的ShardingSphere配置，dataId: " + dataId);
        }

        // 使用从Nacos获取的配置创建数据源（不使用Seata代理）
        dataSource = YamlShardingSphereDataSourceFactory.createDataSource(configContent.getBytes());
        
        // 添加配置监听器，实现动态配置更新
        addConfigListener(dataId);
        
        // 添加中间件集群配置监听器
        addMiddlewareConfigListener();
        
        return dataSource;
    }
    
    /**
     * 添加配置监听器，实现动态配置更新
     * @param dataId 配置ID
     */
    private void addConfigListener(String dataId) {
        try {
            configService.addListener(dataId, group, new Listener() {
                @Override
                public Executor getExecutor() {
                    return null;
                }

                @Override
                public void receiveConfigInfo(String configInfo) {
                    log.info("接收到Nacos配置变更通知，dataId: {}", dataId);
                    try {
                        // 重新创建数据源
                        DataSource newDataSource = YamlShardingSphereDataSourceFactory.createDataSource(configInfo.getBytes());
                        
                        // 安全关闭旧的数据源
                        if (dataSource != null) {
                            try {
                                if (dataSource instanceof ShardingSphereDataSource) {
                                    ((ShardingSphereDataSource) dataSource).close();
                                }
                            } catch (Exception e) {
                                log.warn("关闭旧数据源时发生异常: ", e);
                            }
                        }
                        
                        // 替换旧的数据源
                        dataSource = newDataSource;
                        log.info("成功更新ShardingSphere数据源配置");
                    } catch (Exception e) {
                        log.error("更新ShardingSphere数据源配置失败", e);
                    }
                }
            });
            log.info("已添加Nacos配置监听器，dataId: {}", dataId);
        } catch (Exception e) {
            log.error("添加Nacos配置监听器失败", e);
        }
    }
    
    /**
     * 添加中间件集群配置监听器
     */
    private void addMiddlewareConfigListener() {
        try {
            // 监听中间件集群配置
            String dataId = "middleware-cluster-config.yaml";
            configService.addListener(dataId, group, new Listener() {
                @Override
                public Executor getExecutor() {
                    return null;
                }

                @Override
                public void receiveConfigInfo(String configInfo) {
                    log.info("接收到中间件集群配置变更通知，dataId: {}", dataId);
                    // 这里可以处理中间件集群配置变更
                    // 例如重新初始化连接池等
                }
            });
            log.info("已添加中间件集群配置监听器，dataId: {}", dataId);
        } catch (Exception e) {
            log.error("添加中间件集群配置监听器失败", e);
        }
    }
}