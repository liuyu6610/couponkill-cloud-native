package com.aliyun.seckill.couponkillorderservice.service.tcc.Impl;

import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import com.aliyun.seckill.couponkillorderservice.service.tcc.SeckillTccService;
import io.seata.rm.tcc.api.BusinessActionContext;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

@Service
public class SeckillTccServiceImpl implements SeckillTccService {

    @Autowired
    private OrderMapper orderMapper;

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;

    @Autowired
    private OrderService orderService;

    @Override
    @Transactional
    public boolean prepareSeckill(BusinessActionContext context, Long userId, Long couponId) {
        // 1. 尝试锁定库存
        boolean lockSuccess = couponServiceFeignClient.lockStock(couponId);
        if (!lockSuccess) {
            return false;
        }

        // 2. 创建状态为"准备中"的订单
        Order order = new Order();
        order.setId(context.getXid()); // 使用Seata全局事务ID作为订单ID
        order.setUserId(userId);
        order.setCouponId(couponId);
        order.setStatus(0); // 准备中
        order.setCreateTime(LocalDateTime.now());
        order.setUpdateTime(LocalDateTime.now());
        orderMapper.insert(order);

        return true;
    }

    @Override
    @Transactional
    public boolean commit(BusinessActionContext context) {
        String orderId = context.getXid();
        // 1. 更新订单状态为"已创建"
        orderMapper.updateStatus(orderId, 1, LocalDateTime.now());

        // 2. 确认扣减库存
        Long couponId = Long.parseLong(context.getActionContext("couponId").toString());
        couponServiceFeignClient.confirmDeductStock(couponId);

        return true;
    }

    @Override
    @Transactional
    public boolean rollback(BusinessActionContext context) {
        String orderId = context.getXid();
        Long userId = Long.parseLong(context.getActionContext("userId").toString());
        Long couponId = Long.parseLong(context.getActionContext("couponId").toString());

        // 1. 删除或取消订单
        orderMapper.updateStatus(orderId, 4, LocalDateTime.now()); // 已取消

        // 2. 释放库存
        couponServiceFeignClient.releaseStock(couponId);

        // 3. 发放补偿优惠券
        orderService.handleSeckillFailure(orderId, userId, couponId);

        return true;
    }
}
