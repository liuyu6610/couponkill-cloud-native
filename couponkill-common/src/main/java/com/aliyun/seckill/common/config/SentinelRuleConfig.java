package com.aliyun.seckill.common.config;

import com.alibaba.csp.sentinel.datasource.ReadableDataSource;
import com.alibaba.csp.sentinel.datasource.nacos.NacosDataSource;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRule;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager;
import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.TypeReference;
import com.alibaba.nacos.api.PropertyKeyConst;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import java.util.List;
import java.util.Properties;

@Configuration
public class SentinelRuleConfig {

    // 从Nacos配置中心获取服务地址（bootstrap.yml中配置）
    @Value("${spring.cloud.nacos.config.server-addr}")
    private String nacosServerAddr;

    // 从Nacos配置中心获取命名空间（默认dev）
    @Value("${spring.cloud.nacos.config.namespace:dev}")
    private String nacosNamespace;

    @Bean
    public ReadableDataSource<String, List<FlowRule>> flowRuleDataSource() {
        // Nacos配置参数
        String groupId = "DEFAULT_GROUP"; // 与bootstrap.yml中的group一致
        String dataId = "sentinel-flow-rules"; // Nacos中存储限流规则的dataId

        // 构建Nacos连接属性（包含命名空间）
        Properties properties = new Properties();
        properties.put(PropertyKeyConst.SERVER_ADDR, nacosServerAddr);
        properties.put(PropertyKeyConst.NAMESPACE, nacosNamespace); // 关键：通过Properties配置命名空间

        // 创建Nacos数据源，指定配置监听和规则解析器
        ReadableDataSource<String, List<FlowRule>> dataSource = new NacosDataSource<>(
                properties,  // 传入包含命名空间的Properties
                groupId,
                dataId,
                source -> JSON.parseObject(source, new TypeReference<List<FlowRule>>() {}) // 解析JSON为FlowRule列表
        );

        // 将数据源注册到Sentinel规则管理器
        FlowRuleManager.register2Property(dataSource.getProperty());
        return dataSource;
    }
}