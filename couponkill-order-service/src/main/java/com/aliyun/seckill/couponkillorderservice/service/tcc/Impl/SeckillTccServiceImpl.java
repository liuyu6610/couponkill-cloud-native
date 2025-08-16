// 修改 SeckillTccServiceImpl.java
package com.aliyun.seckill.couponkillorderservice.service.tcc.Impl;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import com.aliyun.seckill.couponkillorderservice.service.tcc.SeckillTccService;
import io.seata.rm.tcc.api.BusinessActionContext;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

@Slf4j
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
        try {
            // 1. 尝试锁定库存
            ApiResponse<Boolean> lockResponse = couponServiceFeignClient.lockStock(couponId);
            boolean lockSuccess = lockResponse != null && lockResponse.getData() != null && lockResponse.getData();
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
            order.setVersion(0);
            orderMapper.insert(order);

            return true;
        } catch (Exception e) {
            log.error("prepareSeckill 失败，userId: {}, couponId: {}", userId, couponId, e);
            return false;
        }
    }

    @Override
    @Transactional
    public boolean commit(BusinessActionContext context) {
        try {
            String orderId = context.getXid();
            // 1. 更新订单状态为"已创建"
            orderMapper.updateStatus(orderId, 1, LocalDateTime.now());

            // 2. 确认扣减库存
            Long couponId = Long.parseLong(context.getActionContext("couponId").toString());
            ApiResponse<Boolean> confirmResponse = couponServiceFeignClient.confirmDeductStock(couponId);
            // 可以根据需要处理返回结果

            return true;
        } catch (Exception e) {
            log.error("commit 失败，context: {}", context, e);
            return false;
        }
    }

    // 在 SeckillTccServiceImpl.java 中完善回滚逻辑
    // 在 SeckillTccServiceImpl.java 中完善回滚逻辑
    @Override
    @Transactional
    public boolean rollback(BusinessActionContext context) {
        try {
            String orderId = context.getXid();
            Long userId = Long.parseLong(context.getActionContext("userId").toString());
            Long couponId = Long.parseLong(context.getActionContext("couponId").toString());

            // 1. 删除或取消订单
            orderMapper.updateStatus(orderId, 4, LocalDateTime.now()); // 已取消

            // 2. 释放库存
            ApiResponse<Boolean> releaseResponse = couponServiceFeignClient.releaseStock(couponId);
            boolean releaseSuccess = releaseResponse != null && releaseResponse.getData() != null && releaseResponse.getData();
            log.info("释放库存结果: couponId={}, result={}", couponId, releaseSuccess);

            // 3. 清理用户领取状态，允许用户重新参与秒杀
            orderService.clearUserReceivedStatus(userId, couponId);

            // 4. 发放补偿优惠券
            orderService.handleSeckillFailure(orderId, userId, couponId);

            return true;
        } catch (Exception e) {
            log.error("rollback 失败，context: {}", context, e);
            return false;
        }
    }
}
