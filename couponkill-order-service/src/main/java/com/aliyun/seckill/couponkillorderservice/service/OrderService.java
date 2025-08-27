package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.pojo.Order;

import java.util.List;
import java.util.concurrent.CompletableFuture;

public interface OrderService {
    /**
     * 创建订单
     * 在创建订单时会从优惠券的所有分片中选择一个有库存的分片，以提高系统QPS
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return 创建的订单
     */
    Order createOrder(Long userId, Long couponId);
    
    /**
     * 取消订单
     * @param orderId 订单ID
     * @param userId 用户ID
     * @return 是否取消成功
     */
    boolean cancelOrder(String orderId, Long userId);
    
    /**
     * 检查用户是否已领取指定优惠券
     * 用于防止用户重复领取同一优惠券
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return 是否已领取
     */
    boolean hasUserReceivedCoupon(Long userId, Long couponId);
    
    /**
     * 检查用户领取优惠券数量限制
     * 包括总优惠券数量限制和秒杀优惠券数量限制
     * @param userId 用户ID
     * @param couponType 优惠券类型 1-常驻, 2-秒抢
     * @return 是否超过限制
     */
    boolean checkCouponCountLimit(Long userId, int couponType);
    
    /**
     * 根据用户ID查询订单列表
     * @param userId 用户ID
     * @param pageNum 页码
     * @param pageSize 每页数量
     * @return 订单列表
     */
    List<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize);
    
    /**
     * 根据条件查询所有订单
     * @param pageNum 页码
     * @param pageSize 每页数量
     * @param startTime 开始时间
     * @param endTime 结束时间
     * @return 订单列表
     */
    List<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime);
    
    /**
     * 保存订单
     * @param order 订单对象
     * @return 保存后的订单
     */
    Order saveOrder(Order order);
    
    /**
     * 更新用户优惠券计数
     * @param userId 用户ID
     * @param couponType 优惠券类型
     * @param change 变化数量
     */
    void updateUserCouponCount(Long userId, int couponType, int change);
    
    /**
     * 检查用户是否在冷却期
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return 是否在冷却期
     */
    boolean checkUserInCooldown(Long userId, Long couponId);
    
    /**
     * 设置用户冷却期
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @param seconds 冷却时间（秒）
     */
    void setUserCooldown(Long userId, Long couponId, int seconds);
    
    /**
     * 处理秒杀失败的补偿逻辑
     * @param orderId 订单ID
     * @param userId 用户ID
     * @param couponId 优惠券ID
     */
    void handleSeckillFailure(String orderId, Long userId, Long couponId);
    
    /**
     * 清理用户领取状态
     * @param userId 用户ID
     * @param couponId 优惠券ID
     */
    void clearUserReceivedStatus(Long userId, Long couponId);
    
    /**
     * 更新订单状态
     * @param orderId 订单ID
     * @param status 状态
     */
    void updateOrderStatus(String orderId, int status);
    
    /**
     * 创建订单（复用方法）
     * @param order 订单对象
     * @return 创建的订单
     */
    Order createOrder(Order order);
    
    /**
     * 根据ID获取订单
     * @param id 订单ID
     * @return 订单对象
     */
    Order getOrderById(Long id);
    
    /**
     * 支付订单
     * @param orderId 订单ID
     * @return 是否支付成功
     */
    boolean payOrder(Long orderId);
    
    /**
     * 获取订单总数
     * @return 订单总数
     */
    long count();

    /**
     * 处理秒杀逻辑
     * 在处理过程中会从优惠券的所有分片中选择一个有库存的分片，以提高系统QPS
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