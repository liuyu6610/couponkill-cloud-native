package com.aliyun.seckill.couponkillorderservice.service.Impl;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import org.springframework.stereotype.Service;

@Service
public class OrderServiceImpl implements OrderService {

    private final OrderMapper orderMapper;

    public OrderServiceImpl(OrderMapper orderMapper) {
        this.orderMapper = orderMapper;
    }

    @Override
    public Order createOrder(Order order) {
        orderMapper.insertOrder(order);
        return order;
    }

    @Override
    public Order getOrderById(Long id) {
        return orderMapper.selectOrderById(id);
    }

    @Override
    public boolean payOrder(Long orderId) {
        return orderMapper.updateOrderStatus(orderId, "PAID") > 0;
    }
}
