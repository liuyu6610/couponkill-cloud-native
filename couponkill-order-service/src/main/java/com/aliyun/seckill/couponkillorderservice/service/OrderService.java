package com.aliyun.seckill.couponkillorderservice.service;


import com.aliyun.seckill.common.pojo.Order;

public interface OrderService {
    Order createOrder(Order order);
    Order getOrderById(Long id);
    boolean payOrder(Long orderId);
}
