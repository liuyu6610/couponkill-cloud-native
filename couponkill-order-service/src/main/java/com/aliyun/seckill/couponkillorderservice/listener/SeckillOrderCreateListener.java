package com.aliyun.seckill.couponkillorderservice.listener;

import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.messaging.handler.annotation.Payload;
import org.springframework.stereotype.Component;

/**
 * 消费秒杀预扣成功后的落单命令（热路径已 Lua 扣 Redis）。
 * 只接受 SeckillOrderCommand；缺字段的毒消息直接丢弃，避免 OrderMessage 混题拖垮消费组。
 */
@Slf4j
@Component
public class SeckillOrderCreateListener {

    @Autowired
    private OrderService orderService;

    @KafkaListener(
            topics = "${kafka.topic.seckill-order-create:seckill_order_create}",
            groupId = "${kafka.consumer.group.seckill-create:seckill-order-create-group}",
            containerFactory = "seckillOrderCommandKafkaListenerContainerFactory"
    )
    public void onMessage(@Payload SeckillOrderCommand command) {
        if (command == null
                || isBlank(command.getRequestId())
                || isBlank(command.getUserId())
                || isBlank(command.getCouponId())) {
            log.warn("丢弃非法/非预扣命令消息（可能是历史 OrderMessage 污染）: {}", command);
            return;
        }
        log.info("收到秒杀落单命令: requestId={}, userId={}, couponId={}",
                command.getRequestId(), command.getUserId(), command.getCouponId());
        orderService.fulfillSeckillOrder(command);
    }

    private static boolean isBlank(String s) {
        return s == null || s.isBlank();
    }
}
