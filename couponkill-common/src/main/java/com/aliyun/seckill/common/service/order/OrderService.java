// com.aliyun.seckill.order.service.OrderService.java
package com.aliyun.seckill.common.service.order;

import com.aliyun.seckill.common.pojo.Order;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.IService;

public interface OrderService extends IService<Order> {
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

    Page<Order> getOrderByUserId (Long userId, Integer pageNum, Integer pageSize);

    Page<Order> getAllOrderByCondition (Integer pageNum, Integer pageSize, String startTime, String endTime);
/**
 * 保存订单
 */
    Order saveOrder (Order order);
}