package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.pojo.Order;

import java.util.List;

public interface OrderService {
    // 原有方法
    Order createOrder(Order order);
    Order getOrderById(Long id);
    boolean payOrder(Long orderId);

    // 新增方法
    Order createOrder(Long userId, Long couponId);
    boolean cancelOrder(String orderId, Long userId);
    boolean hasUserReceivedCoupon(Long userId, Long couponId);
    boolean checkCouponCountLimit(Long userId, int couponType);
    List<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize);
    List<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime);
    Order saveOrder(Order order);
    long count();

    // 用户优惠券计数相关方法
    void updateUserCouponCount(Long userId, int couponType, int change);

    boolean checkUserInCooldown(Long userId, Long couponId);

    void setUserCooldown(Long userId, Long couponId, int cooldownSeconds);

    
    void handleSeckillFailure(String orderId, Long userId, Long couponId);

    void updateOrderStatus(String orderId, int i);
}
