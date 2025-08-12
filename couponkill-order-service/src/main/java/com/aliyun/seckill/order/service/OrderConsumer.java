package com.aliyun.seckill.order.service;

import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import com.aliyun.seckill.order.domain.OrderEntity;
import com.aliyun.seckill.order.domain.OrderRepo;
import jakarta.transaction.Transactional;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.apache.rocketmq.spring.annotation.RocketMQMessageListener;
import org.apache.rocketmq.spring.core.RocketMQListener;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.time.Instant;

@Slf4j
@Component
@RequiredArgsConstructor
@RocketMQMessageListener(topic = "seckill.order.create", consumerGroup = "order-consumer-group")
public class OrderConsumer implements RocketMQListener<SeckillOrderCommand> { // 注意：原代码中接口名可能有误，应为 RocketMQListener
    private final OrderRepo repo;
    private final RestClient.Builder restClientBuilder; // 注入 RestClient 构建器

    // 初始化 couponClient（基于注入的构建器）
    private RestClient couponClient() {
        return restClientBuilder.baseUrl("http://coupon-service:8080/api/v1/seckill").build();
    }

    @Transactional
    public void onMessage(SeckillOrderCommand cmd) {
        try {
            var ex = repo.findByRequestId(cmd.getRequestId()).orElse(null);
            if (ex == null) {
                var order = OrderEntity.builder()
                        .requestId(cmd.getRequestId())
                        .couponId(cmd.getCouponId())
                        .userId(cmd.getUserId())
                        .status("CREATED")
                        .createdAt(Instant.now()).build();
                repo.save(order);
            }
            // 使用注入的 RestClient 调用
            var orderId = repo.findByRequestId(cmd.getRequestId()).map(o -> String.valueOf(o.getId())).orElse("0");
            couponClient().post()
                    .uri("/internal/success?requestId={r}&orderId={o}", cmd.getRequestId(), orderId)
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception e) {
            log.error("order create fail, reqId={}", cmd.getRequestId(), e);
            // 补偿调用
            try {
                couponClient().post()
                        .uri("/internal/compensate?requestId={r}&couponId={c}&userId={u}", cmd.getRequestId(), cmd.getCouponId(), cmd.getUserId())
                        .retrieve()
                        .toBodilessEntity();
            } catch (Exception ex) {
                log.warn("compensate call error", ex);
            }
        }
    }
}
