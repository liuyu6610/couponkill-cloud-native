// SeckillOrderResultListener.java
package com.aliyun.seckill.couponkillorderservice.listener;

import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import com.aliyun.seckill.couponkillorderservice.service.ReservationService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

/**
 * 秒杀订单结果消息监听器（Kafka）
 */
@Component
public class SeckillOrderResultListener {

    @Autowired
    private OrderService orderService;

    @Autowired
    private ReservationService reservationService;

    @KafkaListener(
            topics = "${kafka.topic.seckill-order-result:seckill_order_result}",
            groupId = "${kafka.consumer.group.seckill-result:seckill-order-result-group}",
            containerFactory = "orderMessageKafkaListenerContainerFactory"
    )
    public void onMessage(OrderMessage message) {
        if (message == null) {
            return;
        }
        if ("SUCCESS".equals(message.getStatus())) {
            // 处理成功订单
            orderService.updateOrderStatus(message.getOrderId(), 2); // 已完成
            reservationService.onFulfillResult(
                    message.getUserId(), message.getCouponId(), message.getOrderId(), true);
        } else if ("FAILED".equals(message.getStatus())) {
            // 处理失败订单，进行补偿
            orderService.handleSeckillFailure(
                    message.getOrderId(),
                    message.getUserId(),
                    message.getCouponId()
            );
            reservationService.onFulfillResult(
                    message.getUserId(), message.getCouponId(), message.getOrderId(), false);
        }
    }
}
