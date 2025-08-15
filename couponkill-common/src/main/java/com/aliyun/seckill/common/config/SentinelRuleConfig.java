package com.aliyun.seckill.common.config;

import com.alibaba.csp.sentinel.datasource.ReadableDataSource;
import com.alibaba.csp.sentinel.datasource.nacos.NacosDataSource;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRule;
import com.alibaba.csp.sentinel.slots.block.flow.FlowRuleManager;
import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.TypeReference;
import jakarta.annotation.PostConstruct;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.context.annotation.Configuration;

import java.util.List;

@Configuration
@ConditionalOnProperty(prefix = "spring.cloud.sentinel.datasource.ds1.nacos", name = "server-addr")
public class SentinelRuleConfig {
    @Value("${spring.cloud.sentinel.datasource.ds1.nacos.server-addr:localhost:8848}")
    private String nacosServerAddr;

    @Value("${spring.cloud.sentinel.datasource.ds1.nacos.dataId:${spring.application.name}-sentinel-rules}")
    private String dataId;

    @Value("${spring.cloud.sentinel.datasource.ds1.nacos.groupId:SENTINEL_GROUP}")
    private String groupId;

    @PostConstruct
    public void init() {
        // 从Nacos动态获取限流规则
        ReadableDataSource<String, List<FlowRule>> flowRuleDataSource = new NacosDataSource<>(
                nacosServerAddr, groupId, dataId,
                source -> JSON.parseObject(source, new TypeReference<List<FlowRule>>() {})
        );
        FlowRuleManager.register2Property(flowRuleDataSource.getProperty());
    }
}
