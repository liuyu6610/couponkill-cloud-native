// 文件: couponkill-order-service/src/main/java/com/aliyun/seckill/couponkillorderservice/service/Impl/OrderServiceImpl.java
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
import com.aliyun.seckill.couponkillorderservice.feign.GoSeckillFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import lombok.extern.slf4j.Slf4j;
import org.apache.commons.pool2.BasePooledObjectFactory;
import org.apache.commons.pool2.PooledObject;
import org.apache.commons.pool2.impl.DefaultPooledObject;
import org.apache.commons.pool2.impl.GenericObjectPool;
import org.apache.commons.pool2.impl.GenericObjectPoolConfig;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.messaging.Message;
import org.springframework.messaging.support.MessageBuilder;
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
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
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
    @Autowired
    private GoSeckillFeignClient goSeckillFeignClient;
    // 添加本地缓存
    private final Cache<String, Boolean> localCache = Caffeine.newBuilder()
            .maximumSize(10000)
            .expireAfterWrite(1, TimeUnit.MINUTES)
            .build();

    // 自定义线程池用于异步处理
    private final ExecutorService executorService = Executors.newFixedThreadPool(50);

    /**
     * 批量更新Redis缓存 - 创建订单时使用
     */
    private void batchUpdateRedisOnCreate(Long userId, Long couponId, int couponType) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        String userTotalCountKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String userSeckillCountKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
            // 设置用户领取状态
            connection.set(userReceivedKey.getBytes(), "true".getBytes());
            connection.expire(userReceivedKey.getBytes(), 3600); // 1小时过期

            // 增加用户总优惠券计数
            connection.stringCommands().incr(userTotalCountKey.getBytes());

            // 如果是秒杀优惠券，增加秒杀优惠券计数
            if (couponType == 2) {
                connection.stringCommands().incr(userSeckillCountKey.getBytes());
            }
            return null;
        });
    }
    /**
     * 批量更新Redis缓存 - 取消订单时使用
     */
    private void batchUpdateRedisOnCancel(Long userId, Long couponId, int couponType) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        String userTotalCountKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String userSeckillCountKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
            // 删除用户领取状态
            connection.del(userReceivedKey.getBytes());

            // 减少用户总优惠券计数
            connection.stringCommands().decr(userTotalCountKey.getBytes());

            // 如果是秒杀优惠券，减少秒杀优惠券计数
            if (couponType == 2) {
                connection.stringCommands().decr(userSeckillCountKey.getBytes());
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

    @Override
    @Transactional
    public Order createOrder(Long userId, Long couponId) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;

        try {
            // 1. 使用Lua脚本原子性检查和设置（防止重复秒杀）
            String luaScript =
                    "local key = KEYS[1] " +
                            "local exists = redis.call('EXISTS', key) " +
                            "if exists == 1 then " +
                            "   return 0 " +  // 已存在
                            "else " +
                            "   redis.call('SET', key, '1') " +
                            "   redis.call('EXPIRE', key, 3600) " +
                            "   return 1 " +  // 设置成功
                            "end";
            DefaultRedisScript<Long> redisScript = new DefaultRedisScript<>();
            redisScript.setScriptText(luaScript);
            redisScript.setResultType(Long.class);

            Long result = redisTemplate.execute(redisScript,
                    Collections.singletonList(userReceivedKey));

            if (result == 0) {
                throw new BusinessException(ResultCode.REPEAT_SECKILL);
            }

            // 2. 检查用户限制
            Coupon coupon = getCouponById(couponId);
            if (coupon == null) {
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
            }

            if (!checkCouponCountLimit(userId, coupon.getType())) {
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_LIMIT_EXCEEDED);
            }

            // 3. 调用Coupon服务扣减库存并获取使用的虚拟分片ID
            // 这里直接调用Coupon服务的deductStockWithVirtualId方法
            // 该方法会从优惠券的所有虚拟分片中随机或轮询选择一个有库存的分片进行扣减
            ApiResponse<String> deductResponse = couponServiceFeignClient.deductStockWithVirtualId(couponId);
            String selectedVirtualId = deductResponse != null ? deductResponse.getData() : null;

            if (selectedVirtualId == null || selectedVirtualId.isEmpty()) {
                // 扣减库存失败，清理Redis标记
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_OUT_OF_STOCK);
            }

            // 4. 创建订单
            Order order = new Order();
            order.setId(String.valueOf(idGenerator.nextId()));
            order.setUserId(userId);
            order.setCouponId(couponId);
            order.setVirtualId(selectedVirtualId); // 设置使用的虚拟分片ID

            order.setStatus(1); // 已创建
            order.setGetTime(LocalDateTime.now());
            order.setExpireTime(LocalDateTime.now().plus(coupon.getValidDays(), ChronoUnit.DAYS));
            order.setCreateTime(LocalDateTime.now());
            order.setUpdateTime(LocalDateTime.now());
            order.setRequestId(UUID.randomUUID().toString());
            order.setVersion(0);

            // 插入数据库
            orderMapper.insert(order);

            // 5. 异步处理后续操作
            handlePostOrderCreation(userId, couponId, coupon.getType(), order);

            return order;

        } catch (Exception e) {
            // 清理Redis缓存
            redisTemplate.delete(userReceivedKey);

            if (e instanceof BusinessException) {
                throw (BusinessException) e;
            } else {
                throw new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), "系统错误");
            }
        }
    }

    // 处理订单创建后的后续操作
    private void handlePostOrderCreation(Long userId, Long couponId, int couponType, Order order) {
        try {
            // 并行执行互不依赖的任务
            CompletableFuture<Void> updateUserCountFuture = CompletableFuture.runAsync(() ->
                            updateUserCouponCount(userId, couponType, 1),
                    executorService
            );
            CompletableFuture<Void> updateRedisFuture = CompletableFuture.runAsync(() ->
                            batchUpdateRedisOnCreate(userId, couponId, couponType),
                    executorService
            );
            // 等待并行任务完成后再发送消息
            CompletableFuture.allOf(updateUserCountFuture, updateRedisFuture).join();

            // 发送消息（独立异步操作）
            sendOrderMessage(order, userId, couponId);
        } catch (Exception e) {
            log.error("处理订单后续操作失败", e);
            // 增加失败重试机制
            retryIfNecessary(userId, couponId, couponType, order, e);
        }
    }

    private void sendOrderMessage(Order order, Long userId, Long couponId) {
        OrderMessage orderMessage = null;
        try {
            orderMessage = messagePool.borrowObject();
            orderMessage.setOrderId(order.getId());
            orderMessage.setUserId(userId);
            orderMessage.setCouponId(couponId);
            orderMessage.setCreateTime(new Date());
            orderMessage.setStatus("PENDING");

            // 将OrderMessage包装成Message对象
            Message<OrderMessage> message = MessageBuilder.withPayload(orderMessage).build();

            // 发送事务消息（确保本地事务提交后消息才被消费）
            rocketMQTemplate.sendMessageInTransaction(
                    seckillOrderCreateTopic,
                    message, // 使用包装后的Message对象
                    order // 附加参数，用于事务回调判断
            );
        } catch (Exception e) {
            log.error("获取消息对象失败", e);
        } finally {
            if (orderMessage != null) {
                try {
                    // 重置对象状态后归还
                    orderMessage.setOrderId(null);
                    orderMessage.setUserId(null);
                    orderMessage.setCouponId(null);
                    orderMessage.setCreateTime(null);
                    orderMessage.setStatus(null);
                    messagePool.returnObject(orderMessage);
                } catch (Exception e) {
                    log.error("归还消息对象失败", e);
                }
            }
        }
    }

    // 简单的重试逻辑
    private void retryIfNecessary(Long userId, Long couponId, int couponType, Order order, Exception e) {
        // 这里可以实现更复杂的重试逻辑，比如使用Spring Retry
        log.warn("处理订单后续操作失败，需要重试: userId={}, couponId={}", userId, couponId, e);
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
        try {
            // 使用批量操作更新Redis缓存
            batchUpdateRedisOnCancel(userId, order.getCouponId(), coupon.getType());
        } catch (Exception e) {
            log.warn("更新Redis缓存失败: userId={}, couponId={}", userId, order.getCouponId(), e);
        }
        return true;
    }

    @Override
    public boolean hasUserReceivedCoupon(Long userId, Long couponId) {
        String key = userId + ":" + couponId;
        // 先查本地缓存
        Boolean cached = localCache.getIfPresent(key);
        if (cached != null) {
            return cached;
        }
        // 再查Redis
        String redisKey = USER_RECEIVED_KEY + key;
        Boolean result = redisTemplate.hasKey(redisKey);
        if (result != null && result) {
            localCache.put(key, true);
            return true;
        }

        // 最后查数据库
        long count = orderMapper.countByUserAndCoupon(userId, couponId);
        boolean dbResult = count > 0;

        if (dbResult) {
            localCache.put(key, true);
            redisTemplate.opsForValue().set(redisKey, true, Duration.ofMinutes(30));
        }

        return dbResult;
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

            // 更新缓存，设置较长的过期时间以减少数据库访问
            redisTemplate.opsForValue().set(totalKey, totalCount, 300, TimeUnit.SECONDS);
            redisTemplate.opsForValue().set(seckillKey, seckillCount, 300, TimeUnit.SECONDS);
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
    @Override
    public void handleSeckillFailure(String orderId, Long userId, Long couponId) {
        try {
            log.info("开始处理秒杀失败补偿，orderId={}, userId={}, couponId={}", orderId, userId, couponId);

            // 1. 恢复库存 - 调用Coupon服务的增加库存方法
            try {
                ApiResponse<Boolean> response = couponServiceFeignClient.increaseStock(couponId);
                if (response != null && response.getData() != null && response.getData()) {
                    log.info("恢复库存成功，couponId={}", couponId);
                } else {
                    log.warn("恢复库存可能失败，couponId={}", couponId);
                }
            } catch (Exception e) {
                log.error("调用Coupon服务恢复库存时发生异常，couponId={}", couponId, e);
            }

            // 2. 清理用户领取状态缓存，允许用户重新参与秒杀
            String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
            try {
                redisTemplate.delete(userReceivedKey);
                log.info("清理用户领取状态缓存成功: userId={}, couponId={}", userId, couponId);
            } catch (Exception e) {
                log.warn("清理用户领取状态缓存失败: userId={}, couponId={}", userId, couponId, e);
            }

            // 3. 创建补偿优惠券
            try {
                Coupon compensationCoupon = new Coupon();
                compensationCoupon.setType(1); // 普通优惠券
                compensationCoupon.setFaceValue(BigDecimal.valueOf(compensationAmount)); // 从配置获取
                compensationCoupon.setValidDays(1); // 1天有效期
                compensationCoupon.setStatus(1);
                compensationCoupon.setName("秒杀失败补偿券");
                compensationCoupon.setDescription("由于秒杀失败给予的补偿优惠券");
                compensationCoupon.setMinSpend(BigDecimal.ZERO);
                compensationCoupon.setPerUserLimit(1);

                // 修复：createCompensationCoupon返回的是ApiResponse<Void>
                ApiResponse<Void> response = couponServiceFeignClient.createCompensationCoupon(compensationCoupon);
                if (response != null && response.success()) {
                    log.info("创建补偿优惠券成功");
                } else {
                    log.warn("创建补偿优惠券可能失败");
                }
            } catch (Exception e) {
                log.error("创建补偿优惠券时发生异常", e);
            }

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
        if (id == null) {
            throw new BusinessException(ResultCode.PARAM_ERROR.getCode(), "订单ID不能为空");
        }
        try {
            return orderMapper.selectOrderById(String.valueOf(id));
        } catch (Exception e) {
            log.error("查询订单失败，订单ID: {}", id, e);
            throw new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), "查询订单失败");
        }
    }

    @Override
    @Transactional
    public boolean payOrder(Long orderId) {
        if (orderId == null) {
            throw new BusinessException(ResultCode.PARAM_ERROR.getCode(), "订单ID不能为空");
        }

        try {
            // 1. 查询订单是否存在
            Order order = orderMapper.selectOrderById(String.valueOf(orderId));
            if (order == null) {
                throw new BusinessException(ResultCode.ORDER_NOT_FOUND);
            }

            // 2. 检查订单状态，只有已创建的订单才能支付
            if (order.getStatus() != 1) {
                log.warn("订单状态不正确，无法支付，订单ID: {}, 当前状态: {}", orderId, order.getStatus());
                return false;
            }

            // 3. 更新订单状态为已支付
            LocalDateTime now = LocalDateTime.now();
            order.setStatus(2); // 已使用
            order.setUseTime(now);
            order.setUpdateTime(now);

            // 修复：将Long类型的orderId转换为String类型
            int result = orderMapper.updateOrderStatus(String.valueOf(orderId), 2); // 2表示已使用
            return result > 0;
        } catch (BusinessException e) {
            throw e;
        } catch (Exception e) {
            log.error("支付订单失败，订单ID: {}", orderId, e);
            throw new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), "支付订单失败");
        }
    }


    @Override
    public long count() {
        return 0;
    }

    public Coupon getCouponById(Long couponId) {
        try {
            ApiResponse<Coupon> response = couponServiceFeignClient.getCouponById(couponId);
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

    @Override
    public void handleSeckillSuccess(String orderId, Long userId, Long couponId, String virtualId) {
        try {
            log.info("处理秒杀成功，orderId={}, userId={}, couponId={}, virtualId={}", orderId, userId, couponId, virtualId);

            // 1. 更新用户优惠券计数
            try {
                // 获取优惠券类型
                Coupon coupon = getCouponById(couponId);
                if (coupon != null) {
                    updateUserCouponCount(userId, coupon.getType(), 1);
                    log.info("更新用户优惠券计数成功，userId={}, couponType={}", userId, coupon.getType());
                }
            } catch (Exception e) {
                log.error("更新用户优惠券计数失败，userId={}", userId, e);
            }

            // 2. 设置用户冷却期
            try {
                setUserCooldown(userId, couponId, 2); // 设置2秒冷却期
                log.info("设置用户冷却期成功，userId={}, couponId={}", userId, couponId);
            } catch (Exception e) {
                log.error("设置用户冷却期失败，userId={}, couponId={}", userId, couponId, e);
            }

            // 3. 更新Redis缓存
            try {
                Coupon coupon = getCouponById(couponId);
                if (coupon != null) {
                    batchUpdateRedisOnCreate(userId, couponId, coupon.getType());
                    log.info("更新Redis缓存成功，userId={}, couponId={}", userId, couponId);
                }
            } catch (Exception e) {
                log.error("更新Redis缓存失败，userId={}, couponId={}", userId, couponId, e);
            }

            // 4. 发送订单创建成功消息
            try {
                OrderMessage orderMessage = new OrderMessage();
                orderMessage.setOrderId(orderId);
                orderMessage.setUserId(userId);
                orderMessage.setCouponId(couponId);
                orderMessage.setVirtualId(virtualId);
                orderMessage.setCreateTime(new Date());
                orderMessage.setStatus("SUCCESS");

                asyncSendMessage(orderMessage);
                log.info("发送订单创建成功消息成功，orderId={}", orderId);
            } catch (Exception e) {
                log.error("发送订单创建成功消息失败，orderId={}", orderId, e);
            }
        } catch (Exception e) {
            log.error("处理秒杀成功时出错，orderId={}, userId={}, couponId={}, virtualId={}",
                    orderId, userId, couponId, virtualId, e);
        }
    }

    /**
     * 处理秒杀逻辑
     * @param userId 用户ID
     * @param couponId 优惠券ID
     * @return 是否秒杀成功
     */
    @Override
    @Transactional
    public boolean processSeckill(Long userId, Long couponId) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;

        try {
            // 1. 使用Lua脚本原子性检查和设置（防止重复秒杀）
            String luaScript =
                    "local key = KEYS[1] " +
                            "local exists = redis.call('EXISTS', key) " +
                            "if exists == 1 then " +
                            "   return 0 " +  // 已存在
                            "else " +
                            "   redis.call('SET', key, '1') " +
                            "   redis.call('EXPIRE', key, 3600) " +
                            "   return 1 " +  // 设置成功
                            "end";
            DefaultRedisScript<Long> redisScript = new DefaultRedisScript<>();
            redisScript.setScriptText(luaScript);
            redisScript.setResultType(Long.class);

            Long result = redisTemplate.execute(redisScript,
                    Collections.singletonList(userReceivedKey));

            if (result == 0) {
                throw new BusinessException(ResultCode.REPEAT_SECKILL);
            }

            // 2. 检查用户限制
            Coupon coupon = getCouponById(couponId);
            if (coupon == null) {
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
            }

            if (!checkCouponCountLimit(userId, coupon.getType())) {
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_LIMIT_EXCEEDED);
            }

            // 3. 调用Coupon服务扣减库存并获取使用的虚拟分片ID
            ApiResponse<String> deductResponse = couponServiceFeignClient.deductStockWithVirtualId(couponId);
            String selectedVirtualId = deductResponse != null ? deductResponse.getData() : null;

            if (selectedVirtualId == null || selectedVirtualId.isEmpty()) {
                // 扣减库存失败，清理Redis标记
                redisTemplate.delete(userReceivedKey);
                throw new BusinessException(ResultCode.COUPON_OUT_OF_STOCK);
            }

            // 4. 创建订单
            Order order = new Order();
            order.setId(String.valueOf(idGenerator.nextId()));
            order.setUserId(userId);
            order.setCouponId(couponId);
            order.setVirtualId(selectedVirtualId); // 设置使用的虚拟分片ID

            order.setStatus(1); // 已创建
            order.setGetTime(LocalDateTime.now());
            order.setExpireTime(LocalDateTime.now().plus(coupon.getValidDays(), ChronoUnit.DAYS));
            order.setCreateTime(LocalDateTime.now());
            order.setUpdateTime(LocalDateTime.now());
            order.setRequestId(UUID.randomUUID().toString());
            order.setVersion(0);

            // 插入数据库
            orderMapper.insert(order);

            // 5. 异步处理后续操作
            handlePostOrderCreation(userId, couponId, coupon.getType(), order);

            // 6. 处理秒杀成功后的操作
            handleSeckillSuccess(order.getId(), userId, couponId, selectedVirtualId);

            return true;

        } catch (Exception e) {
            // 清理Redis缓存
            redisTemplate.delete(userReceivedKey);

            if (e instanceof BusinessException) {
                throw (BusinessException) e;
            } else {
                // 处理秒杀失败补偿
                try {
                    handleSeckillFailure(null, userId, couponId);
                } catch (Exception ex) {
                    log.error("秒杀失败补偿处理异常", ex);
                }
                throw new BusinessException(ResultCode.SYSTEM_ERROR.getCode(), "系统错误");
            }
        }
    }


    // 在 OrderServiceImpl 类中添加对象池
    private final GenericObjectPool<OrderMessage> messagePool = new GenericObjectPool<>(
            new BasePooledObjectFactory<OrderMessage>() {
                @Override
                public OrderMessage create() {
                    return new OrderMessage();
                }

                @Override
                public PooledObject<OrderMessage> wrap(OrderMessage obj) {
                    return new DefaultPooledObject<>(obj);
                }

                @Override
                public void passivateObject(PooledObject<OrderMessage> p) throws Exception {
                    // 重置对象状态
                    OrderMessage msg = p.getObject();
                    msg.setOrderId(null);
                    msg.setUserId(null);
                    msg.setCouponId(null);
                    msg.setCreateTime(null);
                    msg.setStatus(null);
                }
            },
            new GenericObjectPoolConfig<OrderMessage>() {{
                setMaxTotal(100);
                setMinIdle(10);
            }}
    );
}
