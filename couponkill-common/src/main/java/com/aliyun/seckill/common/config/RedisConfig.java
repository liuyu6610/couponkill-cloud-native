// 修改 RedisConfig.java
// 文件路径: com/aliyun/seckill/common/config/RedisConfig.java
package com.aliyun.seckill.common.config;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import com.fasterxml.jackson.datatype.jsr310.ser.LocalDateTimeSerializer;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.connection.*;
import org.springframework.data.redis.connection.lettuce.LettuceConnectionFactory;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.StringRedisSerializer;

import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.List;

@Configuration
public class RedisConfig {

    // 单节点配置
    @Value("${spring.data.redis.host:redis}")
    private String redisHost;

    @Value("${spring.data.redis.port:6379}")
    private int redisPort;

    @Value("${spring.data.redis.password:}")
    private String redisPassword;

    // 集群配置
    @Value("${spring.data.redis.cluster.nodes:}")
    private String clusterNodes;

    // 哨兵配置
    @Value("${spring.data.redis.sentinel.master:}")
    private String sentinelMaster;

    @Value("${spring.data.redis.sentinel.nodes:}")
    private String sentinelNodes;

    @Bean
    public ObjectMapper objectMapper() {
        ObjectMapper objectMapper = new ObjectMapper();
        // 注册 JavaTimeModule 以支持 LocalDateTime 等时间类型
        JavaTimeModule javaTimeModule = new JavaTimeModule();
        javaTimeModule.addSerializer(LocalDateTime.class, new LocalDateTimeSerializer(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")));
        objectMapper.registerModule(javaTimeModule);

        // 启用默认类型信息，确保序列化时保留完整类型信息
        objectMapper.activateDefaultTyping(
                LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL
        );
        return objectMapper;
    }

    @Bean
    public RedisConnectionFactory redisConnectionFactory() {
        // 检查是否配置了集群模式
        if (clusterNodes != null && !clusterNodes.isEmpty()) {
            return redisClusterConnectionFactory();
        }
        
        // 检查是否配置了哨兵模式
        if (sentinelMaster != null && !sentinelMaster.isEmpty() && sentinelNodes != null && !sentinelNodes.isEmpty()) {
            return redisSentinelConnectionFactory();
        }
        
        // 默认使用单节点模式
        return redisStandaloneConnectionFactory();
    }

    /**
     * 单节点Redis连接工厂
     */
    private RedisConnectionFactory redisStandaloneConnectionFactory() {
        RedisStandaloneConfiguration config = new RedisStandaloneConfiguration();
        config.setHostName(redisHost);
        config.setPort(redisPort);
        if (redisPassword != null && !redisPassword.isEmpty()) {
            config.setPassword(RedisPassword.of(redisPassword));
        }
        return new LettuceConnectionFactory(config);
    }

    /**
     * Redis集群连接工厂
     */
    private RedisConnectionFactory redisClusterConnectionFactory() {
        RedisClusterConfiguration config = new RedisClusterConfiguration();
        
        // 解析集群节点
        String[] nodes = clusterNodes.split(",");
        for (String node : nodes) {
            if (!node.trim().isEmpty()) {
                String[] parts = node.trim().split(":");
                if (parts.length == 2) {
                    config.clusterNode(parts[0], Integer.parseInt(parts[1]));
                }
            }
        }
        
        if (redisPassword != null && !redisPassword.isEmpty()) {
            config.setPassword(RedisPassword.of(redisPassword));
        }
        
        return new LettuceConnectionFactory(config);
    }

    /**
     * Redis哨兵连接工厂
     */
    private RedisConnectionFactory redisSentinelConnectionFactory() {
        RedisSentinelConfiguration config = new RedisSentinelConfiguration();
        config.master(sentinelMaster);
        
        // 解析哨兵节点
        String[] nodes = sentinelNodes.split(",");
        List<RedisNode> sentinelNodesList = new ArrayList<>();
        for (String node : nodes) {
            if (!node.trim().isEmpty()) {
                String[] parts = node.trim().split(":");
                if (parts.length == 2) {
                    sentinelNodesList.add(new RedisNode(parts[0], Integer.parseInt(parts[1])));
                }
            }
        }
        config.setSentinels(sentinelNodesList);
        
        if (redisPassword != null && !redisPassword.isEmpty()) {
            config.setPassword(RedisPassword.of(redisPassword));
        }
        
        return new LettuceConnectionFactory(config);
    }

    @Bean
    public RedisTemplate<String, Object> redisTemplate(RedisConnectionFactory connectionFactory) {
        RedisTemplate<String, Object> template = new RedisTemplate<>();
        template.setConnectionFactory(connectionFactory);

        // 使用自定义 ObjectMapper 的序列化器
        ObjectMapper customMapper = objectMapper();
        GenericJackson2JsonRedisSerializer serializer = new GenericJackson2JsonRedisSerializer(customMapper);

        // 使用更快的序列化方式
        template.setKeySerializer(new StringRedisSerializer());
        template.setValueSerializer(serializer);
        template.setHashKeySerializer(new StringRedisSerializer());
        template.setHashValueSerializer(serializer);

        // 关闭事务支持秒杀场景
        template.setEnableTransactionSupport(false);

        // 设置连接池参数
        template.afterPropertiesSet();
        return template;
    }
    
    /**
     * 动态更新Redis连接工厂
     * 用于在运行时根据配置变化重新创建连接工厂
     */
    public RedisConnectionFactory updateRedisConnectionFactory(String mode, List<String> nodes, String password, String masterName) {
        switch (mode.toLowerCase()) {
            case "cluster":
                if (nodes != null && !nodes.isEmpty()) {
                    RedisClusterConfiguration clusterConfig = new RedisClusterConfiguration(nodes);
                    if (password != null && !password.isEmpty()) {
                        clusterConfig.setPassword(RedisPassword.of(password));
                    }
                    return new LettuceConnectionFactory(clusterConfig);
                }
                break;
                
            case "sentinel":
                if (masterName != null && !masterName.isEmpty() && nodes != null && !nodes.isEmpty()) {
                    RedisSentinelConfiguration sentinelConfig = new RedisSentinelConfiguration();
                    sentinelConfig.master(masterName);
                    
                    List<RedisNode> sentinelNodesList = new ArrayList<>();
                    for (String node : nodes) {
                        if (!node.trim().isEmpty()) {
                            String[] parts = node.trim().split(":");
                            if (parts.length == 2) {
                                sentinelNodesList.add(new RedisNode(parts[0], Integer.parseInt(parts[1])));
                            }
                        }
                    }
                    sentinelConfig.setSentinels(sentinelNodesList);
                    
                    if (password != null && !password.isEmpty()) {
                        sentinelConfig.setPassword(RedisPassword.of(password));
                    }
                    
                    return new LettuceConnectionFactory(sentinelConfig);
                }
                break;
                
            case "standalone":
            default:
                // 默认使用单节点模式
                RedisStandaloneConfiguration standaloneConfig = new RedisStandaloneConfiguration();
                if (nodes != null && !nodes.isEmpty()) {
                    String[] parts = nodes.get(0).split(":");
                    if (parts.length == 2) {
                        standaloneConfig.setHostName(parts[0]);
                        standaloneConfig.setPort(Integer.parseInt(parts[1]));
                    }
                }
                if (password != null && !password.isEmpty()) {
                    standaloneConfig.setPassword(RedisPassword.of(password));
                }
                return new LettuceConnectionFactory(standaloneConfig);
        }
        
        // 如果没有匹配的配置，返回默认单节点连接
        return redisStandaloneConnectionFactory();
    }
}