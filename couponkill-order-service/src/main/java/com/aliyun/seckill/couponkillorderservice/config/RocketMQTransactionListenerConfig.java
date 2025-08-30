package com.aliyun.seckill.couponkillorderservice.config;

import org.apache.rocketmq.client.producer.TransactionMQProducer;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.context.annotation.Configuration;

import javax.annotation.PostConstruct;

@Configuration
public class RocketMQTransactionListenerConfig {

    @Autowired
    private RocketMQTemplate rocketMQTemplate;

    @Autowired
    private OrderTransactionListener orderTransactionListener;

    @PostConstruct
    public void addTransactionListener() {
        // 获取TransactionMQProducer并设置事务监听器
        if (rocketMQTemplate.getProducer() instanceof TransactionMQProducer) {
            TransactionMQProducer transactionProducer = (TransactionMQProducer) rocketMQTemplate.getProducer();
            transactionProducer.setTransactionListener(orderTransactionListener);
        }
    }
}