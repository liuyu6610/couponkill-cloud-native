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
import org.springframework.kafka.support.serializer.JsonDeserializer;

import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.Executors;

/**
 * Kafka 消费者：落单 listener 使用虚拟线程执行器，提高 IO 密集 fulfill 吞吐。
 */
@Configuration
public class KafkaConsumerConfig {

    @Value("${spring.kafka.bootstrap-servers:kafka:9092}")
    private String bootstrapServers;

    @Value("${couponkill.seckill.listener-concurrency:8}")
    private int seckillListenerConcurrency;

    @Value("${couponkill.seckill.result-listener-concurrency:4}")
    private int resultListenerConcurrency;

    private Map<String, Object> baseConsumerProps() {
        Map<String, Object> props = new HashMap<>();
        props.put(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, bootstrapServers);
        props.put(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        props.put(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, true);
        return props;
    }

    /** 每条消息在虚拟线程上处理，避免平台线程池成为消费背压 */
    private AsyncTaskExecutor virtualListenerExecutor(String namePrefix) {
        return new TaskExecutorAdapter(Executors.newThreadPerTaskExecutor(
                Thread.ofVirtual().name(namePrefix, 0).factory()));
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
