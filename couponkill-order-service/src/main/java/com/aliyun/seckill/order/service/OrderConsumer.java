package com.aliyun.seckill.order.service;

import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import com.aliyun.seckill.order.domain.OrderEntity;
import com.aliyun.seckill.order.domain.OrderRepo;
import jakarta.transaction.Transactional;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.kafka.support.Acknowledgment;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.time.Instant;

@Slf4j
@Component
@RequiredArgsConstructor
public class OrderConsumer {
    private final OrderRepo repo;
    // 内部调用 coupon-service 的回调（可用服务发现/网关）
    private final RestClient couponClient = RestClient.builder().baseUrl("http://coupon-service:8080/api/v1/seckill").build();

    @KafkaListener(topics = "seckill.order.create")
    @Transactional
    public void onMessage(ConsumerRecord<String, SeckillOrderCommand> record, Acknowledgment ack){
        var cmd = record.value();
        try {
            var ex = repo.findByRequestId(cmd.getRequestId()).orElse(null);
            if (ex == null) {
                var order = OrderEntity.builder()
                        .requestId(cmd.getRequestId())
                        .couponId(cmd.getCouponId())
                        .userId(cmd.getUserId())
                        .status("CREATED")
                        .createdAt( Instant.now()).build();
                repo.save(order);
            }
            // 回写成功（以 requestId->orderId）
            var orderId = repo.findByRequestId(cmd.getRequestId()).map(o->String.valueOf(o.getId())).orElse("0");
            couponClient.post().uri("/internal/success?requestId={r}&orderId={o}", cmd.getRequestId(), orderId).retrieve().toBodilessEntity();
            ack.acknowledge();
        } catch (Exception e){
            log.error("order create fail, reqId={}", cmd.getRequestId(), e);
            // 回滚补偿：库存+1 & 发补偿券
            try {
                couponClient.post().uri("/internal/compensate?requestId={r}&couponId={c}&userId={u}", cmd.getRequestId(), cmd.getCouponId(), cmd.getUserId()).retrieve().toBodilessEntity();
            } catch (Exception ex){ log.warn("compensate call error", ex); }
            ack.acknowledge(); // 记审计+DLQ更严谨；这里简化为一次性消费
        }
    }
}
