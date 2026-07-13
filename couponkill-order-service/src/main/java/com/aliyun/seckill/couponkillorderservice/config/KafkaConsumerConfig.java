package com.aliyun.seckill.couponkillorderservice.config;

import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.common.serialization.StringDeserializer;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.task.AsyncTaskExecutor;
import org.springframework.core.task.support.TaskExecutorAdapter;
import org.springframework.kafka.config.ConcurrentKafkaListenerContainerFactory;
import org.springframework.kafka.core.ConsumerFactory;
import org.springframework.kafka.core.DefaultKafkaConsumerFactory;
import org.springframework.kafka.listener.ContainerProperties;
import org.springframework.kafka.support.serializer.JsonDeserializer;

import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.Executors;

/**
 * Kafka 消费者：落单 listener 使用虚拟线程执行器。
 * auto-commit / offset-reset / ack 模式均可从 Nacos（couponkill.seckill.* / spring.kafka.consumer.*）收敛。
 */
@Configuration
public class KafkaConsumerConfig {

    @Value("${spring.kafka.bootstrap-servers:kafka:9092}")
    private String bootstrapServers;

    @Value("${couponkill.seckill.listener-concurrency:8}")
    private int seckillListenerConcurrency;

    @Value("${couponkill.seckill.result-listener-concurrency:4}")
    private int resultListenerConcurrency;

    /** 默认关闭 auto-commit，由 Spring AckMode 显式提交，失败重试语义更清晰 */
    @Value("${spring.kafka.consumer.enable-auto-commit:false}")
    private boolean enableAutoCommit;

    @Value("${spring.kafka.consumer.auto-offset-reset:earliest}")
    private String autoOffsetReset;

    /**
     * RECORD=每条成功后提交；与 enable-auto-commit=false 搭配。
     * 若开启 auto-commit，则退回 BATCH（由消费者定时提交）。
     */
    @Value("${couponkill.seckill.kafka-ack-mode:RECORD}")
    private String ackMode;

    private Map<String, Object> baseConsumerProps() {
        Map<String, Object> props = new HashMap<>();
        props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
        props.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, autoOffsetReset);
        props.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, enableAutoCommit);
        return props;
    }

    private AsyncTaskExecutor virtualListenerExecutor(String namePrefix) {
        return new TaskExecutorAdapter(Executors.newThreadPerTaskExecutor(
                Thread.ofVirtual().name(namePrefix, 0).factory()));
    }

    private void applyAckMode(ConcurrentKafkaListenerContainerFactory<?, ?> factory) {
        if (enableAutoCommit) {
            return;
        }
        try {
            factory.getContainerProperties().setAckMode(ContainerProperties.AckMode.valueOf(ackMode));
        } catch (IllegalArgumentException e) {
            log.warn("非法 kafka-ack-mode={}，回退 RECORD", ackMode);
            factory.getContainerProperties().setAckMode(ContainerProperties.AckMode.RECORD);
        }
    }

    @Bean
    public ConsumerFactory<String, OrderMessage> orderMessageConsumerFactory() {
        JsonDeserializer<OrderMessage> valueDeserializer = new JsonDeserializer<>(OrderMessage.class, false);
        valueDeserializer.addTrustedPackages("com.aliyun.seckill.common.pojo");
        valueDeserializer.setUseTypeHeaders(false);
        return new DefaultKafkaConsumerFactory<>(baseConsumerProps(), new StringDeserializer(), valueDeserializer);
    }

    @Bean
    public ConcurrentKafkaListenerContainerFactory<String, OrderMessage> orderMessageKafkaListenerContainerFactory() {
        ConcurrentKafkaListenerContainerFactory<String, OrderMessage> factory =
                new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(orderMessageConsumerFactory());
        factory.setConcurrency(resultListenerConcurrency);
        factory.getContainerProperties().setListenerTaskExecutor(virtualListenerExecutor("kafka-result-vt-"));
        applyAckMode(factory);
        return factory;
    }

    @Bean
    public ConsumerFactory<String, SeckillOrderCommand> seckillOrderCommandConsumerFactory() {
        JsonDeserializer<SeckillOrderCommand> valueDeserializer =
                new JsonDeserializer<>(SeckillOrderCommand.class, false);
        valueDeserializer.addTrustedPackages("com.aliyun.seckill.common.pojo");
        valueDeserializer.setUseTypeHeaders(false);
        return new DefaultKafkaConsumerFactory<>(baseConsumerProps(), new StringDeserializer(), valueDeserializer);
    }

    @Bean
    public ConcurrentKafkaListenerContainerFactory<String, SeckillOrderCommand>
    seckillOrderCommandKafkaListenerContainerFactory() {
        ConcurrentKafkaListenerContainerFactory<String, SeckillOrderCommand> factory =
                new ConcurrentKafkaListenerContainerFactory<>();
        factory.setConsumerFactory(seckillOrderCommandConsumerFactory());
        factory.setConcurrency(seckillListenerConcurrency);
        factory.getContainerProperties().setListenerTaskExecutor(virtualListenerExecutor("kafka-seckill-vt-"));
        applyAckMode(factory);
        // FixedBackOff(0,0)=不重试毒消息；业务失败由 fulfill 内补偿 + 删除消费锁后允许合法重投
        factory.setCommonErrorHandler(new org.springframework.kafka.listener.DefaultErrorHandler(
                (record, exception) -> log.warn(
                        "秒杀落单消费跳过毒消息: topic={}, offset={}, err={}",
                        record.topic(), record.offset(), exception.toString()),
                new org.springframework.util.backoff.FixedBackOff(0L, 0L)
        ));
        return factory;
    }

    private static final org.slf4j.Logger log =
            org.slf4j.LoggerFactory.getLogger(KafkaConsumerConfig.class);
}
