package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.pojo.Order;

import java.util.List;
import java.util.concurrent.CompletableFuture;

public interface OrderService {
    Order createOrder(Long userId, Long couponId);
    boolean cancelOrder(String orderId, Long userId);
    boolean hasUserReceivedCoupon(Long userId, Long couponId);
    boolean checkCouponCountLimit(Long userId, int couponType);
    List<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize);
    List<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime);
    Order saveOrder(Order order);
    void updateUserCouponCount(Long userId, int couponType, int change);
    boolean checkUserInCooldown(Long userId, Long couponId);
    void setUserCooldown(Long userId, Long couponId, int seconds);
    void handleSeckillFailure(String orderId, Long userId, Long couponId);
    void clearUserReceivedStatus(Long userId, Long couponId);
    void updateOrderStatus(String orderId, int status);
    Order createOrder(Order order);
    Order getOrderById(Long id);
    boolean payOrder(Long orderId);
    long count();

    /**
     * 处理秒杀逻辑
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return 是否秒杀成功
     */
    boolean processSeckill(Long userId, Long couponId);

    /**
     * 处理秒杀成功后的操作
     * @param orderId 订单ID
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @param virtualId 虚拟ID
     */
    void handleSeckillSuccess(String orderId, Long userId, Long couponId, String virtualId);
    
    /**
     * 异步处理秒杀逻辑以提高QPS
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return CompletableFuture包装的处理结果
     */
    default CompletableFuture<Boolean> processSeckillAsync(Long userId, Long couponId) {
        return CompletableFuture.supplyAsync(() -> processSeckill(userId, couponId));
    }
    
    /**
     * 异步处理秒杀成功后的操作以提高QPS
     * @param orderId 订单ID
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @param virtualId 虚拟ID
     * @return CompletableFuture包装的处理结果
     */
    default CompletableFuture<Void> handleSeckillSuccessAsync(String orderId, Long userId, Long couponId, String virtualId) {
        return CompletableFuture.runAsync(() -> handleSeckillSuccess(orderId, userId, couponId, virtualId));
    }
}
