// couponkill-coupon-service/src/main/java/com/aliyun/seckill/coupon/mq/StockUpdateConsumer.java
package com.aliyun.seckill.coupon.mq;

import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.common.service.coupon.CouponService;
import org.apache.rocketmq.spring.annotation.RocketMQMessageListener;
import org.apache.rocketmq.spring.core.RocketMQListener;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Component;

@Component
@RocketMQMessageListener(topic = "order-create-topic", consumerGroup = "coupon-stock-consumer-group")
public class StockUpdateConsumer implements RocketMQListener<OrderMessage> {

    @Autowired
    private CouponService couponService;

    @Override
    public void onMessage(OrderMessage message) {
        // 处理库存更新
        couponService.deductStock(message.getCouponId());
    }
}