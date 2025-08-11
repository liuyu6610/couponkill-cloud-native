package com.aliyun.seckill.order;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.common.service.order.OrderService;
import com.aliyun.seckill.order.feign.GoSeckillFeignClient;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

@Service
public class CreateOrderService {

    private static final Logger log = LoggerFactory.getLogger(CreateOrderService.class);

    @Autowired
    private GoSeckillFeignClient seckillGoClient;

    @Autowired
    private OrderService orderService; // 假设存在OrderService处理订单持久化

    public Order createOrder(Long userId, Long couponId) {
        long threshold = 100; // 自定义阈值
        long current = orderService.count(); // 假设OrderService有count方法
        Order order = new Order();
        order.setUserId(userId);
        order.setCouponId(couponId);

        if (current > threshold) {
            log.info("高并发检测，调用 Go 模块处理秒杀请求");
            // 修复：传递两个必需的参数 userId 和 couponId
            Result<?> result = seckillGoClient.seckill(userId, couponId);
            log.info("Go 模块返回: {}", result);

            // 根据 Go 模块处理结果插入订单（倒序 ID）

            order.setCreatedByGo(1);
            order.setCreatedByJava(0);
            // 使用适当的方法生成ID，这里假设OrderService会处理
            orderService.saveOrder(order); // 假设存在saveOrder方法
            return order;
        }

        // 正常 Java 流程（正序 ID）
        order.setCreatedByJava(1);
        order.setCreatedByGo(0);
        orderService.saveOrder(order);// 假设存在saveOrder方法
        return order;
    }
}
