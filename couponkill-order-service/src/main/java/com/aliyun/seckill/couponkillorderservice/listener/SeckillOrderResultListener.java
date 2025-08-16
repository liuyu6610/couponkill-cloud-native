// SeckillOrderResultListener.java
package com.aliyun.seckill.couponkillorderservice.listener;

import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import org.apache.rocketmq.spring.annotation.ConsumeMode;
import org.apache.rocketmq.spring.annotation.MessageModel;
import org.apache.rocketmq.spring.annotation.RocketMQMessageListener;
import org.apache.rocketmq.spring.core.RocketMQListener;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

/**
 * 秒杀订单结果消息监听器
 */
@Component
@RocketMQMessageListener(
        consumerGroup = "seckill-order-result-group",
        topic = "seckill_order_result",
        selectorExpression = "*",
        consumeMode = ConsumeMode.CONCURRENTLY,
        messageModel = MessageModel.CLUSTERING,
        consumeThreadMax = 64
)
public class SeckillOrderResultListener implements RocketMQListener<OrderMessage> {

    @Autowired
    private OrderService orderService;

    @Override
    public void onMessage(OrderMessage message) {
        if ("SUCCESS".equals(message.getStatus())) {
            // 处理成功订单
            orderService.updateOrderStatus(message.getOrderId(), 2); // 已完成
        } else if ("FAILED".equals(message.getStatus())) {
            // 处理失败订单，进行补偿
            orderService.handleSeckillFailure(
                    message.getOrderId(),
                    message.getUserId(),
                    message.getCouponId()
            );
        }
    }
}
