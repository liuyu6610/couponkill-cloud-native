// 文件路径: com/aliyun/seckill/couponkillorderservice/service/Impl/OrderServiceImpl.java
package com.aliyun.seckill.couponkillorderservice.service.Impl;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import com.aliyun.seckill.common.utils.SnowflakeIdGenerator;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import lombok.extern.slf4j.Slf4j;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.Duration;
import java.time.LocalDateTime;
import java.time.temporal.ChronoUnit;
import java.util.Collections;
import java.util.Date;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

@Slf4j
@Service
public class OrderServiceImpl implements OrderService {

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;

    @Autowired
    private OrderMapper orderMapper;
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    @Autowired
    private RocketMQTemplate rocketMQTemplate;
    // 使用Pipeline批量操作Redis
    private void batchUpdateRedisCache(Long userId, Long couponId, boolean isSeckill) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        String userTotalCountKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String userSeckillCountKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        // 批量操作Redis
        redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
            connection.set(userReceivedKey.getBytes(), "true".getBytes());
            connection incr(userTotalCountKey.getBytes());
            if (isSeckill) {
                connection.incr(userSeckillCountKey.getBytes());
            }
            return null;
        });
    }
    // 异步处理非关键业务逻辑
    @Async("asyncExecutor")
    public void asyncSendMessage(OrderMessage message) {
        try {
            rocketMQTemplate.convertAndSend(seckillOrderCreateTopic, message);
        } catch (Exception e) {
            log.error("异步发送消息失败", e);
        }
    }


    // 在OrderServiceImpl类中添加缺少的字段
    @Value("${couponkill.seckill.compensation-amount:10.0}")
    private double compensationAmount;

    @Value("${rocketmq.topic.seckill-order-create:seckill_order_create}")
    private String seckillOrderCreateTopic;

    @Value("${rocketmq.topic.seckill-compensation:seckill_compensation}")
    private String seckillCompensationTopic;

    // 初始化雪花算法ID生成器
    private final SnowflakeIdGenerator idGenerator = new SnowflakeIdGenerator(1, 1);

    private static final int MAX_TOTAL_COUPONS = 15;
    private static final int MAX_SECKILL_COUPONS = 5;
    private static final String USER_COUPON_COUNT_KEY = "user:coupon:count:";
    private static final String USER_RECEIVED_KEY = "user:received:";

    // 在 OrderServiceImpl.java 中更新消息发送逻辑
    @Override
    @Transactional
    public Order createOrder(Long userId, Long couponId) {
        log.info("开始创建订单，userId: {}, couponId: {}", userId, couponId);

        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;

        try {
            // 1. 先检查Redis缓存（快速失败）
            Boolean userReceived = redisTemplate.hasKey(userReceivedKey);
            if (Boolean.TRUE.equals(userReceived)) {
                throw new BusinessException(ResultCode.REPEAT_SECKILL);
            }

            // 2. 使用Lua脚本原子性检查和设置
            String luaScript =
                    "local key = KEYS[1] " +
                            "local exists = redis.call('EXISTS', key) " +
                            "if exists == 1 then " +
                            "   return 0 " +
                            "else " +
                            "   redis.call('SET', key, '1') " +
                            "   redis.call('EXPIRE', key, 3600) " +
                            "   return 1 " +
                            "end";

            DefaultRedisScript<Long> redisScript = new DefaultRedisScript<>();
            redisScript.setScriptText(luaScript);
            redisScript.setResultType(Long.class);

            Long result = redisTemplate.execute(redisScript,
                    Collections.singletonList(userReceivedKey));

            if (result == 0) {
                throw new BusinessException(ResultCode.REPEAT_SECKILL);
            }

            // 3. 预检查库存（减少远程调用）
            String stockKey = "coupon:stock:" + couponId;
            String stockStr = (String) redisTemplate.opsForValue().get(stockKey);
            if (stockStr != null) {
                int stock = Integer.parseInt(stockStr);
                if (stock <= 0) {
                    throw new BusinessException(ResultCode.COUPON_OUT_OF_STOCK);
                }
            }

            // 4. 扣减库存（使用异步+补偿机制）
            CompletableFuture<Boolean> deductFuture = CompletableFuture.supplyAsync(() -> {
                try {
                    ApiResponse<Boolean> deductResponse = couponServiceFeignClient.deductStock(couponId);
                    return deductResponse != null && deductResponse.getData() != null && deductResponse.getData();
                } catch (Exception e) {
                    log.error("扣减库存失败", e);
                    return false;
                }
            });

            // 5. 异步获取优惠券信息
            CompletableFuture<ApiResponse<Coupon>> couponFuture = CompletableFuture.supplyAsync(() -> {
                return couponServiceFeignClient.getCouponById(couponId);
            });

            // 等待关键操作完成
            boolean deductSuccess = deductFuture.get(3, TimeUnit.SECONDS);
            ApiResponse<Coupon> couponResponse = couponFuture.get(3, TimeUnit.SECONDS);

            if (!deductSuccess) {
                throw new BusinessException(ResultCode.COUPON_OUT_OF_STOCK);
            }

            Coupon coupon = couponResponse != null && couponResponse.getData() != null ?
                    couponResponse.getData() : null;
            if (coupon == null) {
                throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
            }

            // 6. 创建订单（使用批量插入）
            Order order = new Order();
            order.setId(String.valueOf(idGenerator.nextId()));
            order.setUserId(userId);
            order.setCouponId(couponId);
            order.setStatus(1);
            order.setGetTime(LocalDateTime.now());
            order.setExpireTime(LocalDateTime.now().plus(coupon.getValidDays(), ChronoUnit.DAYS));
            order.setCreateTime(LocalDateTime.now());
            order.setUpdateTime(LocalDateTime.now());
            order.setRequestId(UUID.randomUUID().toString());
            order.setVersion(0);

            orderMapper.insert(order);

            // 7. 异步更新用户统计和缓存
            CompletableFuture.runAsync(() -> {
                try {
                    updateUserCouponCount(userId, coupon.getType(), 1);

                    // 更新Redis统计
                    String userTotalCountKey = USER_COUPON_COUNT_KEY + userId + ":total";
                    String userSeckillCountKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

                    redisTemplate.opsForValue().increment(userTotalCountKey);
                    if (coupon.getType() == 2) {
                        redisTemplate.opsForValue().increment(userSeckillCountKey);
                    }
                } catch (Exception e) {
                    log.error("异步更新统计失败", e);
                }
            });

            // 8. 异步发送消息
            CompletableFuture.runAsync(() -> {
                try {
                    OrderMessage orderMessage = new OrderMessage();
                    orderMessage.setOrderId(order.getId());
                    orderMessage.setUserId(userId);
                    orderMessage.setCouponId(couponId);
                    orderMessage.setCreateTime(new Date());
                    orderMessage.setStatus("PENDING");
                    asyncSendMessage(orderMessage);
                } catch (Exception e) {
                    log.error("异步发送消息失败", e);
                }
            });

            return order;

        } catch (Exception e) {
            // 清理Redis缓存
            redisTemplate.delete(userReceivedKey);

            // 异步回滚库存
            CompletableFuture.runAsync(() -> {
                try {
                    couponServiceFeignClient.increaseStock(couponId);
                } catch (Exception ex) {
                    log.error("回滚库存失败", ex);
                }
            });

            if (e instanceof BusinessException) {
                throw (BusinessException) e;
            } else {
                throw new BusinessException(ResultCode.SYSTEM_ERROR);
            }
        }
    }



    // 异步发送消息的方法
    private void sendMessageAsync(Order order, Long userId, Long couponId) {
        CompletableFuture.runAsync(() -> {
            try {
                OrderMessage orderMessage = new OrderMessage();
                orderMessage.setOrderId(order.getId());
                orderMessage.setUserId(userId);
                orderMessage.setCouponId(couponId);
                orderMessage.setCreateTime(new Date());
                orderMessage.setStatus("PENDING");

                rocketMQTemplate.convertAndSend(seckillOrderCreateTopic, orderMessage);
                log.info("成功发送秒杀订单创建消息，订单ID: {}", order.getId());
            } catch (Exception e) {
                log.error("发送秒杀订单创建消息时发生异常，订单ID: {}", order.getId(), e);
            }
        });
    }


    /**
     * 清理Redis数据
     */
    private void cleanupRedisData(Long userId, Long couponId, String userReceivedKey,
                                  String userTotalCountKey, String userSeckillCountKey) {
        try {
            redisTemplate.delete(userReceivedKey);
            redisTemplate.opsForValue().decrement(userTotalCountKey);

            // 获取优惠券信息以确定是否为秒杀优惠券
            ApiResponse<Coupon> couponResponse = couponServiceFeignClient.getCouponById(couponId);
            Coupon coupon = couponResponse != null && couponResponse.getData() != null ? couponResponse.getData() : null;
            if (coupon != null && coupon.getType() == 2) {
                redisTemplate.opsForValue().decrement(userSeckillCountKey);
            }
            log.info("清理Redis缓存数据完成: userId={}, couponId={}", userId, couponId);
        } catch (Exception e) {
            log.error("清理Redis缓存数据失败: userId={}, couponId={}", userId, couponId, e);
        }
    }






    @Override
    @Transactional
    public boolean cancelOrder(String orderId, Long userId) {
        // 1. 查询订单
        Order order = orderMapper.selectById(orderId);
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
        order.setUpdateTime(LocalDateTime.now());
        // 修改为新的 updateStatus 方法
        boolean updateSuccess = orderMapper.updateStatusWithCancelTime(orderId, 4, LocalDateTime.now(), LocalDateTime.now()) > 0;
        if (!updateSuccess) {
            return false;
        }

        // 4. 恢复优惠券库存 (通过Feign调用)
        couponServiceFeignClient.increaseStock(order.getCouponId());

        // 5. 获取优惠券类型
        ApiResponse<Coupon> couponResponse = couponServiceFeignClient.getCouponById(order.getCouponId());
        Coupon coupon = couponResponse != null && couponResponse.getData() != null ? couponResponse.getData() : null;
        if (coupon == null || coupon.getType() == null) {
            return false;
        }

        // 6. 更新用户优惠券数量统计
        updateUserCouponCount(userId, coupon.getType(), -1);

        // 7. 更新Redis缓存
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + order.getCouponId();
        String userTotalCountKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String userSeckillCountKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        try {
            // 清理用户领取状态缓存
            redisTemplate.delete(userReceivedKey);

            // 减少用户优惠券计数
            redisTemplate.opsForValue().decrement(userTotalCountKey);
            if (coupon.getType() == 2) { // 秒杀优惠券
                redisTemplate.opsForValue().decrement(userSeckillCountKey);
            }
        } catch (Exception e) {
            log.warn("更新Redis缓存失败: userId={}, couponId={}", userId, order.getCouponId(), e);
        }

        return true;
    }



    @Override
    public boolean hasUserReceivedCoupon(Long userId, Long couponId) {
        String key = USER_RECEIVED_KEY + userId + ":" + couponId;

        try {
            // 先检查Redis缓存
            Object cachedObj = redisTemplate.opsForValue().get(key);
            if (cachedObj != null) {
                // 确保类型正确
                if (cachedObj instanceof Boolean) {
                    return (Boolean) cachedObj;
                } else {
                    // 类型不匹配，删除错误的键
                    redisTemplate.delete(key);
                }
            }

            // 缓存未命中或类型不匹配，查数据库
            long count = orderMapper.countByUserAndCoupon(userId, couponId);
            boolean result = count > 0;

            // 更新缓存
            try {
                if (result) {
                    redisTemplate.opsForValue().set(key, true);
                } else {
                    redisTemplate.opsForValue().set(key, false, Duration.ofMinutes(5)); // 短期缓存
                }
            } catch (Exception redisException) {
                log.warn("更新Redis缓存时出错，userId={}, couponId={}", userId, couponId, redisException);
                // Redis操作失败不影响主流程
            }

            return result;
        } catch (Exception e) {
            log.warn("查询用户领取状态时出错，userId={}, couponId={}", userId, couponId, e);
            // 出错时保守处理，查询数据库
            try {
                long count = orderMapper.countByUserAndCoupon(userId, couponId);
                return count > 0;
            } catch (Exception dbException) {
                log.error("查询数据库时也出错，userId={}, couponId={}", userId, couponId, dbException);
                // 数据库也出错时，默认返回false，避免重复领取
                return true; // 保守处理，假设已领取
            }
        }
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
            UserCouponCount count = orderMapper.selectUserCouponCountById(userId);
            if (count == null) {
                totalCount = 0;
                seckillCount = 0;
                // 初始化用户优惠券数量
                UserCouponCount newCount = new UserCouponCount();
                newCount.setUserId(userId);
                newCount.setTotalCount(0);
                newCount.setSeckillCount(0);
                newCount.setNormalCount(0);
                newCount.setExpiredCount(0); // 确保设置 expiredCount
                orderMapper.insertUserCouponCount(newCount);
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
    public List<Order> getOrderByUserId(Long userId, Integer pageNum, Integer pageSize) {
        int offset = (pageNum - 1) * pageSize;
        return orderMapper.selectByUserId(userId, offset, pageSize);
    }

    @Override
    public List<Order> getAllOrderByCondition(Integer pageNum, Integer pageSize, String startTime, String endTime) {
        int offset = (pageNum - 1) * pageSize;
        return orderMapper.selectAllByCondition(startTime, endTime, offset, pageSize);
    }

    @Override
    public Order saveOrder(Order order) {
        if (order.getId() == null) {
            order.setId(String.valueOf(idGenerator.nextId())); // 使用雪花算法生成ID
        }
        // 设置创建时间
        if (order.getCreateTime() == null) {
            order.setCreateTime(LocalDateTime.now());
        }
        // 确保 version 字段不为 null
        if (order.getVersion() == null) {
            order.setVersion(0);
        }
        order.setUpdateTime(LocalDateTime.now());
        orderMapper.insert(order);
        return order;
    }



    @Override
    @Transactional
    public void updateUserCouponCount(Long userId, int couponType, int change) {
        // 直接使用 orderMapper 处理用户优惠券计数
        UserCouponCount count = orderMapper.selectUserCouponCountById(userId);
        if (count == null) {
            // 初始化逻辑
            count = new UserCouponCount();
            count.setUserId(userId);
            count.setTotalCount(change > 0 ? change : 0);
            count.setSeckillCount((couponType == 2 && change > 0) ? change : 0);
            count.setNormalCount((couponType != 2 && change > 0) ? change : 0);
            count.setExpiredCount(0); // 确保设置 expiredCount
            orderMapper.insertUserCouponCount(count);
        } else {
            // 更新逻辑
            int newTotalCount = count.getTotalCount() + change;
            int newSeckillCount = count.getSeckillCount();
            int newNormalCount = count.getNormalCount();

            if (couponType == 2) { // 秒杀优惠券
                newSeckillCount += change;
            } else { // 普通优惠券
                newNormalCount += change;
            }

            // 确保数值不会为负数
            newTotalCount = Math.max(0, newTotalCount);
            newSeckillCount = Math.max(0, newSeckillCount);
            newNormalCount = Math.max(0, newNormalCount);

            orderMapper.updateUserCouponCount(userId, newTotalCount, newSeckillCount);
        }
    }




    /**
     * 检查用户是否在冷却期
     */
    @Override
    public boolean checkUserInCooldown(Long userId, Long couponId) {
        String key = "seckill:cooldown:" + userId + ":" + couponId;
        return redisTemplate.hasKey(key);
    }

    /**
     * 设置用户冷却期
     */
    @Override
    public void setUserCooldown(Long userId, Long couponId, int seconds) {
        String key = "seckill:cooldown:" + userId + ":" + couponId;
        redisTemplate.opsForValue().set(key, "1", Duration.ofSeconds(seconds));
    }
    /**
     * 处理秒杀失败补偿
     */
    /**
     * 处理秒杀失败补偿
     */
    @Override
    public void handleSeckillFailure(String orderId, Long userId, Long couponId) {
        try {
            // 1. 恢复库存
            couponServiceFeignClient.increaseStock(couponId);

            // 2. 清理用户领取状态缓存，允许用户重新参与秒杀
            String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
            try {
                redisTemplate.delete(userReceivedKey);
            } catch (Exception e) {
                log.warn("清理用户领取状态缓存失败: userId={}, couponId={}", userId, couponId, e);
            }

            // 3. 创建补偿优惠券
            Coupon compensationCoupon = new Coupon();
            compensationCoupon.setUserId(userId);
            compensationCoupon.setType(1); // 普通优惠券
            compensationCoupon.setAmount(BigDecimal.valueOf(compensationAmount)); // 从配置获取
            compensationCoupon.setValidDays(1); // 1天有效期
            compensationCoupon.setStatus(1);
            couponServiceFeignClient.createCompensationCoupon(compensationCoupon);

            // 4. 发送补偿结果消息（用于监控和对账）
            OrderMessage compensationMsg = new OrderMessage();
            compensationMsg.setOrderId(orderId);
            compensationMsg.setUserId(userId);
            compensationMsg.setCouponId(couponId);
            compensationMsg.setStatus("COMPENSATED"); // 状态：已补偿
            compensationMsg.setCreateTime(new Date());

            try {
                // 检查 RocketMQTemplate 是否正确初始化
                if (rocketMQTemplate != null) {
                    rocketMQTemplate.convertAndSend(seckillCompensationTopic, compensationMsg);
                    log.info("发送补偿消息成功，订单ID: {}", orderId);
                } else {
                    log.warn("RocketMQTemplate 未正确初始化，跳过发送补偿消息");
                }
            } catch (Exception e) {
                log.error("发送补偿消息时发生异常，订单ID: {}", orderId, e);
            }
        } catch (Exception e) {
            log.error("处理秒杀失败补偿时出错，orderId={}, userId={}, couponId={}", orderId, userId, couponId, e);
        }
    }

    @Override
    public void clearUserReceivedStatus(Long userId, Long couponId) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        try {
            redisTemplate.delete(userReceivedKey);
            log.info("已清理用户领取状态缓存: userId={}, couponId={}", userId, couponId);
        } catch (Exception e) {
            log.error("清理用户领取状态缓存失败: userId={}, couponId={}", userId, couponId, e);
        }
    }



    @Override
public void updateOrderStatus(String orderId, int status) {
    // 更新订单状态
    orderMapper.updateStatus(orderId, status, LocalDateTime.now());
}

@Override
public Order createOrder(Order order) {
    // 复用已有的保存订单方法
    return saveOrder(order);
}

@Override
public Order getOrderById(Long id) {
    // 注意：由于订单ID是String类型，此方法可能需要调整
    // 根据实际情况决定是否需要实现
    return orderMapper.selectById(String.valueOf(id));
}

@Override
public boolean payOrder(Long orderId) {
    // 更新订单状态为已支付
    int result = orderMapper.updateStatus(String.valueOf(orderId), 3, LocalDateTime.now());
    return result > 0;
}
    @Override
    public long count() {
        return 0;
    }
    public Coupon getCouponById(Long couponId) {
        log.info("开始调用 coupon-service 获取优惠券信息，couponId: {}", couponId);
        try {
            ApiResponse<Coupon> response = couponServiceFeignClient.getCouponById(couponId);
            log.info("从 coupon-service 获取到的响应: {}", response);
            if (response != null && response.getData() != null) {
                log.info("从 coupon-service 获取到的优惠券对象: {}", response.getData());
                return response.getData();
            } else {
                log.warn("从 coupon-service 获取到的优惠券对象为空");
                return null;
            }
        } catch (Exception e) {
            log.error("调用 coupon-service 获取优惠券信息失败", e);
            throw new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), "获取优惠券信息失败");
        }
    }

}
