// com.aliyun.seckill.order.service.impl.OrderServiceImpl.java
package com.aliyun.seckill.common.service.order;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.mapper.OrderMapper;
import com.aliyun.seckill.common.mapper.UserCouponCountMapper;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import com.aliyun.seckill.common.service.coupon.CouponService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import io.seata.spring.annotation.GlobalTransactional;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.time.temporal.ChronoUnit;
import java.util.Date;
import java.util.UUID;

@Service
public class OrderServiceImpl extends ServiceImpl<OrderMapper, Order> implements OrderService {

    @Autowired
    private CouponService couponService;

    @Autowired
    private UserCouponCountMapper userCouponCountMapper;

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;
    @Autowired
    private RocketMQTemplate rocketMQTemplate;

    private static final int MAX_TOTAL_COUPONS = 15;
    private static final int MAX_SECKILL_COUPONS = 5;
    private static final String USER_COUPON_COUNT_KEY = "user:coupon:count:";
    private static final String USER_RECEIVED_KEY = "user:received:";

    @Override
    @GlobalTransactional(rollbackFor = Exception.class)
    public Order createOrder(Long userId, Long couponId) {
        // 1. 获取优惠券信息
        Coupon coupon = couponService.getCouponById(couponId);
        if (coupon == null) {
            throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
        }

        // 2. 检查用户是否已领取该优惠券
        if (hasUserReceivedCoupon(userId, couponId)) {
            throw new BusinessException(ResultCode.REPEAT_SECKILL);
        }

        // 3. 检查用户优惠券数量限制
        if (!checkCouponCountLimit(userId, coupon.getType())) {
            throw new BusinessException(ResultCode.COUPON_LIMIT_EXCEEDED);
        }

        // 4. 扣减优惠券库存
        boolean deductSuccess = couponService.deductStock(couponId);
        if (!deductSuccess) {
            throw new BusinessException(ResultCode.COUPON_OUT_OF_STOCK);
        }

        // 5. 创建订单
        Order order = new Order();
        order.setId(UUID.randomUUID().toString());
        order.setUserId(userId);
        order.setCouponId(couponId);
        order.setStatus(1); // 已创建
        order.setGetTime(LocalDateTime.now());
        order.setExpireTime(LocalDateTime.now().plus(coupon.getValidDays(), ChronoUnit.DAYS));
        save(order);

        // 6. 更新用户优惠券数量统计
        updateUserCouponCount(userId, coupon.getType(), 1);

        // 7. 更新Redis缓存
        redisTemplate.opsForValue().set(USER_RECEIVED_KEY + userId + ":" + couponId, true);
        redisTemplate.opsForValue().increment(USER_COUPON_COUNT_KEY + userId + ":total");
        if (coupon.getType() == 2) { // 秒杀优惠券
            redisTemplate.opsForValue().increment(USER_COUPON_COUNT_KEY + userId + ":seckill");
        }
        // 发送订单创建成功消息
        OrderMessage orderMessage = new OrderMessage();
        orderMessage.setOrderId(order.getId());
        orderMessage.setUserId(userId);
        orderMessage.setCouponId(couponId);
        orderMessage.setCreateTime(new Date());

        rocketMQTemplate.convertAndSend("order-create-topic", orderMessage);

        return order;
    }

    @Override
    @GlobalTransactional(rollbackFor = Exception.class)
    public boolean cancelOrder(String orderId, Long userId) {
        // 1. 查询订单
        Order order = getById(orderId);
        if (order == null || !order.getUserId().equals(userId)) {
            throw new BusinessException(ResultCode.ORDER_NOT_FOUND);
        }

        // 2. 检查订单状态
        if (order.getStatus() != 1) { // 非已创建状态不能取消
            return false;
        }

        // 3. 更新订单状态
        order.setStatus(4); // 已取消
        order.setCancelTime(LocalDateTime.now());
        boolean updateSuccess = updateById(order);
        if (!updateSuccess) {
            return false;
        }

        // 4. 恢复优惠券库存
        couponService.increaseStock(order.getCouponId());

        // 5. 获取优惠券类型
        Coupon coupon = couponService.getCouponById(order.getCouponId());
        if (coupon == null) {
            return false;
        }

        // 6. 更新用户优惠券数量统计
        updateUserCouponCount(userId, coupon.getType(), -1);

        // 7. 更新Redis缓存
        redisTemplate.delete(USER_RECEIVED_KEY + userId + ":" + order.getCouponId());
        redisTemplate.opsForValue().decrement(USER_COUPON_COUNT_KEY + userId + ":total");
        if (coupon.getType() == 2) { // 秒杀优惠券
            redisTemplate.opsForValue().decrement(USER_COUPON_COUNT_KEY + userId + ":seckill");
        }

        return true;
    }

    @Override
    public boolean hasUserReceivedCoupon(Long userId, Long couponId) {
        // 先查缓存
        String key = USER_RECEIVED_KEY + userId + ":" + couponId;
        Boolean hasReceived = (Boolean) redisTemplate.opsForValue().get(key);
        if (Boolean.TRUE.equals(hasReceived)) {
            return true;
        }

        // 缓存未命中，查数据库
        QueryWrapper<Order> queryWrapper = new QueryWrapper<>();
        queryWrapper.eq("user_id", userId)
                .eq("coupon_id", couponId)
                .in("status", 1, 2); // 已创建或已使用状态视为已领取
        long count = count(queryWrapper);
        if (count > 0) {
            redisTemplate.opsForValue().set(key, true);
            return true;
        }

        return false;
    }

    @Override
    public boolean checkCouponCountLimit(Long userId, int couponType) {
        // 先查缓存
        String totalKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String seckillKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        Integer totalCount = (Integer) redisTemplate.opsForValue().get(totalKey);
        Integer seckillCount = (Integer) redisTemplate.opsForValue().get(seckillKey);

        // 缓存未命中，查数据库
        if (totalCount == null || seckillCount == null) {
            UserCouponCount count = userCouponCountMapper.selectById(userId);
            if (count == null) {
                totalCount = 0;
                seckillCount = 0;
                // 初始化用户优惠券数量
                UserCouponCount newCount = new UserCouponCount();
                newCount.setUserId(userId);
                newCount.setTotalCount(0);
                newCount.setSeckillCount(0);
                userCouponCountMapper.insert(newCount);
            } else {
                totalCount = count.getTotalCount();
                seckillCount = count.getSeckillCount();
            }

            // 更新缓存
            redisTemplate.opsForValue().set(totalKey, totalCount);
            redisTemplate.opsForValue().set(seckillKey, seckillCount);
        }

        // 检查限制
        if (totalCount >= MAX_TOTAL_COUPONS) {
            return false;
        }

        if (couponType == 2 && seckillCount >= MAX_SECKILL_COUPONS) {
            return false;
        }

        return true;
    }

    @Override
    public Page<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize) {
        Page<Order> page = new Page<>(pageNum, pageSize);
        QueryWrapper<Order> queryWrapper = new QueryWrapper<>();
        queryWrapper.eq("user_id", userId);
        return page(page, queryWrapper);
    }

    @Override
    public Page<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime) {
        Page<Order> page = new Page<>(pageNum, pageSize);
        QueryWrapper<Order> queryWrapper = new QueryWrapper<>();

        if (startTime != null && !startTime.isEmpty()) {
            queryWrapper.ge("create_time", startTime);
        }
        if (endTime != null && !endTime.isEmpty()) {
            queryWrapper.le("create_time", endTime);
        }

        return page(page, queryWrapper);
    }

    @Override
    public void saveOrder (Order order) {

    }

    @Transactional
    public void updateUserCouponCount(Long userId, int couponType, int change) {
        UserCouponCount count = userCouponCountMapper.selectById(userId);
        if (count == null) {
            count = new UserCouponCount();
            count.setUserId(userId);
            count.setTotalCount(0);
            count.setSeckillCount(0);
        }

        count.setTotalCount(count.getTotalCount() + change);
        if (couponType == 2) { // 秒杀优惠券
            count.setSeckillCount(count.getSeckillCount() + change);
        }

        if (count.getId() == null) {
            userCouponCountMapper.insert(count);
        } else {
            userCouponCountMapper.updateById(count);
        }
    }
}