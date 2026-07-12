// 修改 RedisConfig.java
// 文件路径: com/aliyun/seckill/common/config/RedisConfig.java
package com.aliyun.seckill.common.config;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import com.fasterxml.jackson.datatype.jsr310.ser.LocalDateTimeSerializer;
import io.lettuce.core.ClientOptions;
import io.lettuce.core.SocketOptions;
import io.lettuce.core.TimeoutOptions;
import org.apache.commons.pool2.impl.GenericObjectPoolConfig;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.connection.RedisClusterConfiguration;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.connection.RedisNode;
import org.springframework.data.redis.connection.RedisPassword;
import org.springframework.data.redis.connection.RedisSentinelConfiguration;
import org.springframework.data.redis.connection.RedisStandaloneConfiguration;
import org.springframework.data.redis.connection.lettuce.LettuceConnectionFactory;
import org.springframework.data.redis.connection.lettuce.LettucePoolingClientConfiguration;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.StringRedisSerializer;

import java.time.Duration;
import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.List;

/**
 * Redis 连接工厂：显式接入 Lettuce 连接池与命令超时，避免手写 factory 绕过 spring.data.redis.lettuce.pool。
 */
@Configuration
public class RedisConfig {

    @Value("${spring.data.redis.host:redis}")
    private String redisHost;

    @Value("${spring.data.redis.port:6379}")
    private int redisPort;

    @Value("${spring.data.redis.password:}")
    private String redisPassword;

    @Value("${spring.data.redis.cluster.nodes:}")
    private String clusterNodes;

    @Value("${spring.data.redis.sentinel.master:}")
    private String sentinelMaster;

    @Value("${spring.data.redis.sentinel.nodes:}")
    private String sentinelNodes;

    @Value("${spring.data.redis.lettuce.pool.max-active:64}")
    private int poolMaxActive;

    @Value("${spring.data.redis.lettuce.pool.max-idle:32}")
    private int poolMaxIdle;

    @Value("${spring.data.redis.lettuce.pool.min-idle:8}")
    private int poolMinIdle;

    @Value("${spring.data.redis.lettuce.pool.max-wait:2000ms}")
    private Duration poolMaxWait;

    @Value("${spring.data.redis.timeout:2s}")
    private Duration commandTimeout;

    @Value("${spring.data.redis.connect-timeout:2s}")
    private Duration connectTimeout;

    @Bean
    public ObjectMapper objectMapper() {
        ObjectMapper objectMapper = new ObjectMapper();
        JavaTimeModule javaTimeModule = new JavaTimeModule();
        javaTimeModule.addSerializer(LocalDateTime.class,
                new LocalDateTimeSerializer(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss")));
        objectMapper.registerModule(javaTimeModule);
        objectMapper.activateDefaultTyping(
                LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL
        );
        return objectMapper;
    }

    @Bean
    public RedisConnectionFactory redisConnectionFactory() {
        if (clusterNodes != null && !clusterNodes.isEmpty()) {
            return new LettuceConnectionFactory(buildClusterConfig(), lettuceClientConfiguration());
        }
        if (sentinelMaster != null && !sentinelMaster.isEmpty()
                && sentinelNodes != null && !sentinelNodes.isEmpty()) {
            return new LettuceConnectionFactory(buildSentinelConfig(), lettuceClientConfiguration());
        }
        return new LettuceConnectionFactory(buildStandaloneConfig(), lettuceClientConfiguration());
    }

    private LettucePoolingClientConfiguration lettuceClientConfiguration() {
        GenericObjectPoolConfig<io.lettuce.core.api.StatefulConnection<?, ?>> poolConfig =
                new GenericObjectPoolConfig<>();
        poolConfig.setMaxTotal(poolMaxActive);
        poolConfig.setMaxIdle(poolMaxIdle);
        poolConfig.setMinIdle(poolMinIdle);
        poolConfig.setMaxWait(poolMaxWait);
        poolConfig.setTestOnBorrow(true);

        SocketOptions socketOptions = SocketOptions.builder()
                .connectTimeout(connectTimeout)
                .keepAlive(true)
                .build();
        ClientOptions clientOptions = ClientOptions.builder()
                .socketOptions(socketOptions)
                .timeoutOptions(TimeoutOptions.enabled(commandTimeout))
                .autoReconnect(true)
                .build();

        return LettucePoolingClientConfiguration.builder()
                .poolConfig(poolConfig)
                .clientOptions(clientOptions)
                .commandTimeout(commandTimeout)
                .build();
    }

    private RedisStandaloneConfiguration buildStandaloneConfig() {
        RedisStandaloneConfiguration config = new RedisStandaloneConfiguration();
        config.setHostName(redisHost);
        config.setPort(redisPort);
        if (redisPassword != null && !redisPassword.isEmpty()) {
            config.setPassword(RedisPassword.of(redisPassword));
        }
        return config;
    }

    private RedisClusterConfiguration buildClusterConfig() {
        RedisClusterConfiguration config = new RedisClusterConfiguration();
        for (String node : clusterNodes.split(",")) {
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
        return config;
    }

    private RedisSentinelConfiguration buildSentinelConfig() {
        RedisSentinelConfiguration config = new RedisSentinelConfiguration();
        config.master(sentinelMaster);
        List<RedisNode> sentinelNodesList = new ArrayList<>();
        for (String node : sentinelNodes.split(",")) {
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
        return config;
    }

    @Bean
    public RedisTemplate<String, Object> redisTemplate(RedisConnectionFactory connectionFactory) {
        RedisTemplate<String, Object> template = new RedisTemplate<>();
        template.setConnectionFactory(connectionFactory);
        ObjectMapper customMapper = objectMapper();
        GenericJackson2JsonRedisSerializer serializer = new GenericJackson2JsonRedisSerializer(customMapper);
        template.setKeySerializer(new StringRedisSerializer());
        template.setValueSerializer(serializer);
        template.setHashKeySerializer(new StringRedisSerializer());
        template.setHashValueSerializer(serializer);
        template.setEnableTransactionSupport(false);
        template.afterPropertiesSet();
        return template;
    }

    @Bean
    public org.springframework.data.redis.core.StringRedisTemplate stringRedisTemplate(
            RedisConnectionFactory connectionFactory) {
        return new org.springframework.data.redis.core.StringRedisTemplate(connectionFactory);
    }
}
