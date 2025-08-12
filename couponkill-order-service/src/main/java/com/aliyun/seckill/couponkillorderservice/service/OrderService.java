// 创建包: com.aliyun.seckill.couponkillorderservice.service.order
package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.pojo.Order;

import java.util.List;

public interface OrderService {
    /**
     * 创建订单(领取优惠券)
     */
    Order createOrder(Long userId, Long couponId);

    /**
     * 取消订单(退还优惠券)
     */
    boolean cancelOrder(String orderId, Long userId);

    /**
     * 检查用户是否已领取该优惠券
     */
    boolean hasUserReceivedCoupon(Long userId, Long couponId);

    /**
     * 检查用户优惠券数量是否超过限制
     */
    boolean checkCouponCountLimit(Long userId, int couponType);

    /**
     * 根据用户ID获取订单列表
     */
    List<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize);

    /**
     * 根据条件获取所有订单
     */
    List<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime);

    /**
     * 保存订单
     */
    Order saveOrder(Order order);

    /**
     * 统计订单数量
     */
    long count();
}
