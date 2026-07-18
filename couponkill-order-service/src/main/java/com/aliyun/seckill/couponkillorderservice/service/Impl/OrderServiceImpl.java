// 文件: couponkill-order-service/src/main/java/com/aliyun/seckill/couponkillorderservice/service/Impl/OrderServiceImpl.java
package com.aliyun.seckill.couponkillorderservice.service.Impl;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.common.pojo.Order;
import com.aliyun.seckill.common.pojo.OrderMessage;
import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import com.aliyun.seckill.common.utils.SnowflakeIdGenerator;
import com.aliyun.seckill.common.util.Backoff;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.feign.GoSeckillFeignClient;
import com.aliyun.seckill.couponkillorderservice.feign.UserServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.OrderMapper;
import com.aliyun.seckill.couponkillorderservice.service.AsyncSeckillEnterService;
import com.aliyun.seckill.couponkillorderservice.service.OrderService;
import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import lombok.extern.slf4j.Slf4j;
import org.apache.commons.pool2.BasePooledObjectFactory;
import org.apache.commons.pool2.PooledObject;
import org.apache.commons.pool2.impl.DefaultPooledObject;
import org.apache.commons.pool2.impl.GenericObjectPool;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.LocalDateTime;
import java.time.temporal.ChronoUnit;
import java.util.Collections;
import java.util.Date;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;
import java.util.concurrent.TimeUnit;

@Slf4j
@Service
public class OrderServiceImpl implements OrderService {

    // 在类的字段部分添加messagePool定义
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
                    OrderMessage message = p.getObject();
                    message.setOrderId(null);
                    message.setUserId(null);
                    message.setCouponId(null);
                    message.setVirtualId(null);
                    message.setCreateTime(null);
                    message.setStatus(null);
                }
            }
    );

    @Autowired
    private CouponServiceFeignClient couponServiceFeignClient;
    @Autowired
    private OrderMapper orderMapper;
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;
    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;
    @Autowired
    private GoSeckillFeignClient goSeckillFeignClient;
    
    // 添加用户服务Feign客户端（用于更新用户优惠券统计）
    @Autowired
    private UserServiceFeignClient userServiceFeignClient;

    @Autowired
    private AsyncSeckillEnterService asyncSeckillEnterService;

    @Autowired
    private org.springframework.data.redis.core.StringRedisTemplate stringRedisTemplate;

    // 本地缓存 - 用户优惠券计数
    private final Cache<String, UserCouponCount> userCouponCountCache = Caffeine.newBuilder()
            .maximumSize(10_000)
            .expireAfterWrite(Duration.ofMinutes(10))
            .build();

    // 本地缓存 - 用户冷却状态
    private final Cache<String, Boolean> userCooldownCache = Caffeine.newBuilder()
            .maximumSize(100_000)
            .expireAfterWrite(Duration.ofSeconds(10))
            .build();

    // 本地缓存 - 用户领取状态
    private final Cache<String, Boolean> localCache = Caffeine.newBuilder()
            .maximumSize(100_000)
            .expireAfterWrite(Duration.ofSeconds(30))
            .build();

    /** 后置异步任务执行器：默认虚拟线程，避免 FixedThreadPool(10) 成为隐藏背压 */
    @Autowired
    @Qualifier("asyncExecutor")
    private Executor asyncExecutor;

    /**
     * 批量更新Redis缓存 - 创建订单时：仅维护领取标记。
     * 计数由 updateUserCouponCount 单一写点负责，禁止在此对 total/seckill 再 incr。
     */
    private void batchUpdateRedisOnCreate(Long userId, Long couponId, int couponType) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
            connection.set(userReceivedKey.getBytes(), "true".getBytes());
            connection.expire(userReceivedKey.getBytes(), 3600);
            return null;
        });
    }

    /**
     * 批量更新Redis缓存 - 取消订单时：仅清理领取标记。
     * 计数由 updateUserCouponCount 单一写点负责，禁止在此对 total/seckill 再 decr。
     */
    private void batchUpdateRedisOnCancel(Long userId, Long couponId, int couponType) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        redisTemplate.delete(userReceivedKey);
    }

    // 异步处理非关键业务逻辑
    /**
     * 订单生命周期通知。严禁发到 seckill_order_create（该 topic 只承载 SeckillOrderCommand）。
     */
    @Async("asyncExecutor")
    public void asyncSendMessage(OrderMessage message) {
        try {
            String key = message.getOrderId() != null ? message.getOrderId() : UUID.randomUUID().toString();
            kafkaTemplate.send(orderCreatedTopic, key, message);
        } catch (Exception e) {
            log.error("异步发送订单生命周期消息失败", e);
        }
    }

    @Value("${couponkill.seckill.compensation-amount:10.0}")
    private double compensationAmount;

    @Value("${kafka.topic.seckill-order-result:seckill_order_result}")
    private String seckillOrderResultTopic;

    @Value("${kafka.topic.seckill-compensation:seckill_compensation}")
    private String seckillCompensationTopic;

    /** 订单已落库后的生命周期/审计通知（与秒杀预扣命令 topic 隔离） */
    @Value("${kafka.topic.order-created:order_created}")
    private String orderCreatedTopic;

    /** 消费侧 Structured Concurrency（JDK25 preview）；默认关，开启需 --enable-preview */
    @Value("${couponkill.seckill.structured-concurrency:false}")
    private boolean structuredConcurrency;

    // 初始化雪花算法ID生成器（多实例动态 workerId，见 SnowflakeConfig）
    @Autowired
    private SnowflakeIdGenerator idGenerator;

    private static final int MAX_TOTAL_COUPONS = 15;
    private static final int MAX_SECKILL_COUPONS = 5;
    private static final String USER_COUPON_COUNT_KEY = "user:coupon:count:";
    private static final String USER_RECEIVED_KEY = "user:received:";

    /**
     * 创建订单。Feign / sleep 绝不进入本地事务；仅 order 插入是本地写。
     * 扣库存成功但落库失败时回补库存并清理 Redis 占位。
     */
    @Override
    public Order createOrder(Long userId, Long couponId) {
        String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
        boolean redisMarked = false;
        boolean stockDeducted = false;
        String selectedVirtualId = null;

        try {
            // 1. Redis 原子占位（防重复领取）
            if (!tryMarkUserReceived(userReceivedKey)) {
                log.warn("用户 {} 重复领取优惠券 {}", userId, couponId);
                throw new BusinessException(ErrorCodes.REPEAT_SECKILL, "不可重复秒杀");
            }
            redisMarked = true;

            // 2. 业务校验（本地读 / Feign 读，无事务）
            Coupon coupon = getCouponById(couponId);
            if (coupon == null) {
                log.warn("未找到优惠券: couponId={}", couponId);
                throw new BusinessException(ErrorCodes.COUPON_NOT_FOUND, "优惠券不存在");
            }
            // 秒杀券禁止走同步 /order/create，避免与热路径双重扣库存语义混用
            if (coupon.getType() != null && coupon.getType() == 2) {
                throw new BusinessException(ErrorCodes.SECKILL_USE_DEDICATED_API, "秒杀券请使用 /api/v1/order/seckill 接口");
            }
            if (!checkCouponCountLimit(userId, coupon.getType())) {
                log.warn("用户 {} 达到优惠券领取限制，优惠券类型: {}", userId, coupon.getType());
                throw new BusinessException(ErrorCodes.COUPON_LIMIT_EXCEEDED, "优惠券数量已达上限");
            }

            // 3. 普通券：扣 remaining_stock（非秒杀分片路径）
            if (!deductNormalStockWithRetry(couponId)) {
                log.warn("扣减普通优惠券库存失败: couponId={}", couponId);
                throw new BusinessException(ErrorCodes.OUT_OF_STOCK, "优惠券已抢完");
            }
            stockDeducted = true;
            selectedVirtualId = String.valueOf(couponId);

            // 4. 落库订单（单条 insert，无需外层事务包住 RPC）
            Order order = buildNewOrder(userId, couponId, selectedVirtualId, coupon);
            try {
                orderMapper.insert(order);
            } catch (Exception e) {
                if (isDuplicateEntryException(e)) {
                    log.warn("用户 {} 重复领取优惠券 {}，数据库唯一性约束违反", userId, couponId);
                    throw new BusinessException(ErrorCodes.REPEAT_SECKILL, "不可重复秒杀");
                }
                throw e;
            }

            // 5. 后置异步（计数 / Redis / MQ），失败不回滚已成功订单
            handlePostOrderCreation(userId, couponId, coupon.getType(), order);

            log.info("成功创建订单: orderId={}, userId={}, couponId={}", order.getId(), userId, couponId);
            return order;

        } catch (Exception e) {
            if (redisMarked) {
                redisTemplate.delete(userReceivedKey);
            }
            if (stockDeducted) {
                safeIncreaseNormalStock(couponId);
            }
            log.error("创建订单失败: userId={}, couponId={}", userId, couponId, e);
            if (e instanceof BusinessException) {
                throw (BusinessException) e;
            }
            throw new BusinessException(ErrorCodes.SYS_ERROR, "系统错误");
        }
    }

    private boolean tryMarkUserReceived(String userReceivedKey) {
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
        Long result = redisTemplate.execute(redisScript, Collections.singletonList(userReceivedKey));
        return result != null && result == 1L;
    }

    private boolean deductNormalStockWithRetry(Long couponId) {
        ApiResponse<Boolean> deductResponse = couponServiceFeignClient.deductStock(couponId);
        Boolean ok = deductResponse != null ? deductResponse.getData() : null;
        int retryCount = 0;
        final int maxRetries = 3;
        while ((ok == null || !ok) && retryCount < maxRetries) {
            Backoff.linear(retryCount, 50L, 200L);
            if (Thread.currentThread().isInterrupted()) {
                break;
            }
            deductResponse = couponServiceFeignClient.deductStock(couponId);
            ok = deductResponse != null ? deductResponse.getData() : null;
            retryCount++;
        }
        return ok != null && ok;
    }

    private void safeIncreaseNormalStock(Long couponId) {
        try {
            couponServiceFeignClient.increaseStock(couponId);
            log.info("普通订单失败后已回补库存: couponId={}", couponId);
        } catch (Exception ex) {
            log.error("普通订单失败后回补库存失败，需人工对账: couponId={}", couponId, ex);
        }
    }

    private Order buildNewOrder(Long userId, Long couponId, String selectedVirtualId, Coupon coupon) {
        Order order = new Order();
        order.setId(String.valueOf(idGenerator.nextId()));
        order.setUserId(userId);
        order.setCouponId(couponId);
        order.setVirtualId(selectedVirtualId);
        order.setStatus(1);
        order.setGetTime(LocalDateTime.now());
        order.setExpireTime(LocalDateTime.now().plus(coupon.getValidDays(), ChronoUnit.DAYS));
        order.setCreateTime(LocalDateTime.now());
        order.setUpdateTime(LocalDateTime.now());
        order.setRequestId(UUID.randomUUID().toString());
        order.setVersion(0);
        return order;
    }

    private void safeIncreaseSeckillStock(String virtualId, Long couponId) {
        try {
            if (virtualId != null && !virtualId.isBlank()) {
                couponServiceFeignClient.increaseSeckillStockByShardId(virtualId);
                log.info("订单失败后已按分片回补秒杀库存: virtualId={}", virtualId);
            } else {
                log.error("缺少 virtualId，无法精确回补秒杀库存，需人工对账: couponId={}", couponId);
            }
        } catch (Exception ex) {
            log.error("订单失败后回补秒杀库存失败，需人工对账: virtualId={}, couponId={}", virtualId, couponId, ex);
        }
    }

    // 处理订单创建后的后续操作
    /** @param awaitCritical true=同步等待用户计数/Redis；false=异步 fire-and-forget（Kafka 消费侧，避免占满 listener） */
    private void handlePostOrderCreation(Long userId, Long couponId, int couponType, Order order) {
        handlePostOrderCreation(userId, couponId, couponType, order, true);
    }

    private void handlePostOrderCreation(Long userId, Long couponId, int couponType, Order order,
                                         boolean awaitCritical) {
        try {
            CompletableFuture<Void> updateUserCountFuture = CompletableFuture.runAsync(() ->
                            updateUserCouponCount(userId, couponType, 1),
                    asyncExecutor
            );
            CompletableFuture<Void> updateRedisFuture = CompletableFuture.runAsync(() ->
                            batchUpdateRedisOnCreate(userId, couponId, couponType),
                    asyncExecutor
            );
            CompletableFuture<Void> sendMessageFuture = CompletableFuture.runAsync(() ->
                            sendOrderMessage(order, userId, couponId),
                    asyncExecutor
            );

            CompletableFuture<Void> critical = CompletableFuture.allOf(updateUserCountFuture, updateRedisFuture);
            if (awaitCritical) {
                critical.join();
            } else {
                critical.whenComplete((ok, ex) -> {
                    if (ex != null) {
                        log.error("异步订单后置关键任务失败（可观测/将重试）: orderId={}, userId={}, couponId={}",
                                order.getId(), userId, couponId, ex);
                        Exception asEx = ex instanceof Exception ? (Exception) ex : new Exception(ex);
                        retryIfNecessary(userId, couponId, couponType, order, asEx);
                    }
                });
            }

            sendMessageFuture.exceptionally(throwable -> {
                log.error("异步发送订单消息失败: orderId={}", order.getId(), throwable);
                return null;
            });
        } catch (Exception e) {
            log.error("处理订单后续操作失败", e);
            retryIfNecessary(userId, couponId, couponType, order, e);
        }
    }

    private void sendOrderMessage(Order order, Long userId, Long couponId) {
        OrderMessage orderMessage = null;
        try {
            // 从对象池中借用对象
            orderMessage = messagePool.borrowObject();
            orderMessage.setOrderId(order.getId());
            orderMessage.setUserId(userId);
            orderMessage.setCouponId(couponId);
            orderMessage.setCreateTime(new Date());
            orderMessage.setStatus("PENDING");

            // Kafka 生产者在 send() 内同步完成序列化，随后归还对象到池是安全的。
            // 订单本地事务已提交，这里直接发送创建消息（原 RocketMQ 事务消息回查恒为 COMMIT，属无操作）。
            kafkaTemplate.send(orderCreatedTopic, order.getId(), orderMessage);
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
            throw new BusinessException(ErrorCodes.ORDER_NOT_FOUND, "订单不存在");
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

        // 异步执行非关键操作
        CompletableFuture.runAsync(() -> {
            try {
                ApiResponse<Coupon> couponResponse = couponServiceFeignClient.getCouponById(order.getCouponId());
                Coupon coupon = couponResponse != null && couponResponse.getData() != null ? couponResponse.getData() : null;

                // 4. 按券类型回补库存：秒杀走分片；普通券走 remaining_stock
                if (coupon != null && coupon.getType() != null && coupon.getType() == 2) {
                    if (order.getVirtualId() != null && !order.getVirtualId().isBlank()) {
                        couponServiceFeignClient.increaseSeckillStockByShardId(order.getVirtualId());
                    } else {
                        log.error("取消秒杀订单缺少 virtualId，无法精确回补: orderId={}, couponId={}",
                                orderId, order.getCouponId());
                    }
                } else {
                    couponServiceFeignClient.increaseStock(order.getCouponId());
                }

                // 5. 获取优惠券类型后更新用户券计数
                if (coupon == null || coupon.getType() == null) {
                    log.warn("获取优惠券信息失败，无法更新用户优惠券数量: couponId={}", order.getCouponId());
                    return;
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
            } catch (Exception e) {
                log.error("异步处理订单取消后续操作失败: orderId={}, userId={}", orderId, userId, e);
            }
        }, asyncExecutor);

        return true;
    }

    @Override
    public boolean hasUserReceivedCoupon(Long userId, Long couponId) {
        String key = USER_RECEIVED_KEY + userId + ":" + couponId;

        try {
            // 先查缓存
            Boolean hasReceived = (Boolean) redisTemplate.opsForValue().get(key);
            if (hasReceived != null) {
                return hasReceived;
            }
            
            // 缓存未命中，使用布隆过滤器判断是否存在
            String bloomFilterKey = "user:received:bloom";
            Boolean mightContain = redisTemplate.opsForValue().getBit(bloomFilterKey, getUserIdCouponHash(userId, couponId));
            if (mightContain != null && !mightContain) {
                // 布隆过滤器确定不存在，直接返回false，避免查询数据库
                // 对于确定不存在的键，也设置短时间缓存防止缓存穿透
                redisTemplate.opsForValue().set(key, false, Duration.ofMinutes(5));
                return false;
            }
            
            // 布隆过滤器可能存在，查询数据库确认
            long count = orderMapper.countByUserAndCoupon(userId, couponId);
            boolean result = count > 0;
            
            // 更新缓存，设置较长时间的过期时间
            redisTemplate.opsForValue().set(key, result, Duration.ofMinutes(30));
            
            // 如果用户确实领取过，添加到布隆过滤器中
            if (result) {
                redisTemplate.opsForValue().setBit(bloomFilterKey, getUserIdCouponHash(userId, couponId), true);
            }
            
            return result;
        } catch (Exception e) {
            log.error("查询用户是否已领取优惠券时出错: userId={}, couponId={}", userId, couponId, e);
            // 出错时保守处理，认为用户未领取
            return false;
        }
    }
    
    /**
     * 生成用户ID和优惠券ID的哈希值，用于布隆过滤器
     */
    private long getUserIdCouponHash(Long userId, Long couponId) {
        // 使用简单的哈希算法，实际项目中可以使用更复杂的哈希函数
        long hash = userId * 31 + couponId;
        // 确保hash值为正数
        return Math.abs(hash) % (1024 * 1024 * 8); // 假设布隆过滤器有1M大小
    }

    @Override
    public boolean checkCouponCountLimit(Long userId, int couponType) {
        String totalKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String seckillKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        // Redis 可能序列化为 Integer / Long，统一经 Number 取值
        Integer totalCount = toInteger(redisTemplate.opsForValue().get(totalKey));
        Integer seckillCount = toInteger(redisTemplate.opsForValue().get(seckillKey));

        if (totalCount == null || seckillCount == null) {
            try {
                ApiResponse<UserCouponCount> response = userServiceFeignClient.getUserCouponCount(userId);
                if (response == null || !response.success() || response.getData() == null) {
                    // 失败关闭：拿不到真值则拒绝领取，禁止默认 0 绕过限领
                    log.warn("获取用户优惠券计数失败(空响应)，拒绝领取: userId={}", userId);
                    return false;
                }
                UserCouponCount count = response.getData();
                totalCount = count.getTotalCount() != null ? count.getTotalCount() : 0;
                seckillCount = count.getSeckillCount() != null ? count.getSeckillCount() : 0;
                redisTemplate.opsForValue().set(totalKey, totalCount, 300, TimeUnit.SECONDS);
                redisTemplate.opsForValue().set(seckillKey, seckillCount, 300, TimeUnit.SECONDS);
            } catch (Exception e) {
                log.warn("获取用户优惠券计数异常，拒绝领取: userId={}", userId, e);
                return false;
            }
        }

        if (totalCount >= MAX_TOTAL_COUPONS) {
            log.info("用户 {} 达到总优惠券数量上限: {}", userId, totalCount);
            return false;
        }
        if (couponType == 2 && seckillCount >= MAX_SECKILL_COUPONS) {
            log.info("用户 {} 达到秒杀优惠券数量上限: {}", userId, seckillCount);
            return false;
        }
        return true;
    }

    private static Integer toInteger(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number) {
            return ((Number) value).intValue();
        }
        try {
            return Integer.parseInt(String.valueOf(value));
        } catch (NumberFormatException e) {
            return null;
        }
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
        try {
            // 通过Feign调用用户服务更新用户优惠券统计
            if (change != 0) {
                ApiResponse<Void> response;
                if (couponType == 2) { // 秒杀优惠券
                    response = userServiceFeignClient.updateSeckillCouponCount(userId, change);
                } else { // 普通优惠券
                    response = userServiceFeignClient.updateNormalCouponCount(userId, change);
                }
                
                if (response == null || !response.success()) {
                    log.warn("更新用户优惠券计数失败: userId={}, couponType={}, change={}", userId, couponType, change);
                    throw new BusinessException(ErrorCodes.SYS_ERROR, "更新用户优惠券计数失败");
                }
                
                log.debug("成功更新用户优惠券计数: userId={}, couponType={}, change={}", userId, couponType, change);
            }
        } catch (Exception e) {
            log.error("更新用户优惠券计数异常: userId={}, couponType={}, change={}", userId, couponType, change, e);
            throw new BusinessException(ErrorCodes.SYS_ERROR, "更新用户优惠券计数异常");
        }
        
        // 同时更新Redis中的计数
        String totalKey = USER_COUPON_COUNT_KEY + userId + ":total";
        String seckillKey = USER_COUPON_COUNT_KEY + userId + ":seckill";

        try {
            // 更新Redis中的计数
            if (change != 0) {
                redisTemplate.opsForValue().increment(totalKey, change);

                if (couponType == 2) { // 秒杀优惠券
                    redisTemplate.opsForValue().increment(seckillKey, change);
                }
            }
        } catch (Exception e) {
            log.warn("更新用户优惠券计数缓存失败: userId={}, change={}", userId, change, e);
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

            // 1. 恢复秒杀分片库存：若有订单则按 virtualId；否则仅打日志待人工（无分片信息不可乱写 remaining_stock）
            try {
                if (orderId != null) {
                    Order order = orderMapper.selectById(orderId);
                    if (order != null && order.getVirtualId() != null && !order.getVirtualId().isBlank()) {
                        ApiResponse<Boolean> response =
                                couponServiceFeignClient.increaseSeckillStockByShardId(order.getVirtualId());
                        if (response != null && Boolean.TRUE.equals(response.getData())) {
                            log.info("恢复秒杀分片库存成功，virtualId={}", order.getVirtualId());
                        } else {
                            log.warn("恢复秒杀分片库存可能失败，virtualId={}", order.getVirtualId());
                        }
                    } else {
                        log.error("秒杀失败补偿缺少 virtualId，需人工对账: orderId={}, couponId={}", orderId, couponId);
                    }
                } else {
                    log.error("秒杀失败补偿无 orderId/virtualId，跳过库存回补以免写错字段: couponId={}", couponId);
                }
            } catch (Exception e) {
                log.error("调用Coupon服务恢复秒杀库存时发生异常，couponId={}", couponId, e);
            }

            // 2. 清理用户领取状态缓存，允许用户重新参与秒杀
            String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
            try {
                redisTemplate.delete(userReceivedKey);
                log.info("清理用户领取状态缓存成功: userId={}, couponId={}", userId, couponId);
            } catch (Exception e) {
                log.warn("清理用户领取状态缓存失败: userId={}, couponId={}", userId, couponId, e);
            }

            // 3. 发送补偿结果消息（用于监控和对账）
            OrderMessage compensationMsg = new OrderMessage();
            compensationMsg.setOrderId(orderId);
            compensationMsg.setUserId(userId);
            compensationMsg.setCouponId(couponId);
            compensationMsg.setStatus("COMPENSATED"); // 状态：已补偿
            compensationMsg.setCreateTime(new Date());

            try {
                // 检查 KafkaTemplate 是否正确初始化
                if (kafkaTemplate != null) {
                    kafkaTemplate.send(seckillCompensationTopic, orderId, compensationMsg);
                    log.info("发送补偿消息成功，订单ID: {}", orderId);
                } else {
                    log.warn("KafkaTemplate 未正确初始化，跳过发送补偿消息");
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
            throw new BusinessException(ErrorCodes.INVALID_REQ, "订单ID不能为空");
        }
        try {
            // 使用现有的selectById方法替代已删除的selectOrderById方法
            return orderMapper.selectById(String.valueOf(id));
        } catch (Exception e) {
            log.error("查询订单失败，订单ID: {}", id, e);
            throw new BusinessException(ErrorCodes.SYS_ERROR, "查询订单失败");
        }
    }

    @Override
    @Transactional
    public boolean payOrder(Long orderId) {
        if (orderId == null) {
            throw new BusinessException(ErrorCodes.INVALID_REQ, "订单ID不能为空");
        }

        try {
            // 1. 查询订单是否存在
            // 使用现有的selectById方法替代已删除的selectOrderById方法
            Order order = orderMapper.selectById(String.valueOf(orderId));
            if (order == null) {
                throw new BusinessException(ErrorCodes.ORDER_NOT_FOUND, "订单不存在");
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

            // 使用现有的updateStatus方法替代已删除的updateOrderStatus方法
            int result = orderMapper.updateStatus(String.valueOf(orderId), 2, now); // 2表示已使用
            return result > 0;
        } catch (BusinessException e) {
            throw e;
        } catch (Exception e) {
            log.error("支付订单失败，订单ID: {}", orderId, e);
            throw new BusinessException(ErrorCodes.SYS_ERROR, "支付订单失败");
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
                log.debug("从 coupon-service 获取券元数据: couponId={}", couponId);
                return response.getData();
            } else {
                log.warn("从 coupon-service 获取到的优惠券对象为空");
                return null;
            }
        } catch (Exception e) {
            log.error("调用 coupon-service 获取优惠券信息失败", e);
            throw new BusinessException(ErrorCodes.SYS_ERROR, "获取优惠券信息失败");
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
     * 秒杀热路径：Lua 预扣 Redis + Kafka 异步落单（无同步 Feign 扣库存）。
     * 若券配置了活动时间窗，则开售前/结束后拒绝入队（不改 Lua 返回码语义）。
     */
    @Override
    public EnterSeckillResp enterSeckillAsync(Long userId, Long couponId) {
        Coupon coupon = getCouponById(couponId);
        if (coupon != null
                && coupon.getSeckillStartAt() != null
                && coupon.getSeckillEndAt() != null) {
            LocalDateTime now = LocalDateTime.now();
            if (now.isBefore(coupon.getSeckillStartAt())) {
                return EnterSeckillResp.builder()
                        .status("REJECTED")
                        .err(ErrorCodes.NOT_STARTED)
                        .message("活动未开始，请预约帮抢或稍后再试")
                        .build();
            }
            if (now.isAfter(coupon.getSeckillEndAt())) {
                return EnterSeckillResp.builder()
                        .status("REJECTED")
                        .err(ErrorCodes.ACTIVITY_ENDED)
                        .message("活动已结束")
                        .build();
            }
        }
        return asyncSeckillEnterService.enter(userId, couponId);
    }

    /**
     * 消费 seckill_order_create：DB 对齐扣减 + 落单。Redis 已在热路径扣过。
     */
    @Override
    public void fulfillSeckillOrder(SeckillOrderCommand command) {
        if (command == null || command.getRequestId() == null
                || command.getUserId() == null || command.getCouponId() == null) {
            log.warn("非法秒杀落单命令: {}", command);
            return;
        }
        long userId;
        long couponId;
        try {
            userId = Long.parseLong(command.getUserId());
            couponId = Long.parseLong(command.getCouponId());
        } catch (NumberFormatException e) {
            log.error("秒杀命令 ID 非法: {}", command);
            return;
        }
        String requestId = command.getRequestId();

        // 消费幂等：同一 requestId 只处理一次（防 Kafka 至少一次投递）
        String consumeLockKey = "seckill:consume:" + requestId;
        Boolean firstConsume = stringRedisTemplate.opsForValue()
                .setIfAbsent(consumeLockKey, "1", Duration.ofMinutes(30));
        if (Boolean.FALSE.equals(firstConsume)) {
            log.debug("秒杀落单消费幂等跳过: requestId={}", requestId);
            return;
        }

        if (hasUserReceivedCoupon(userId, couponId)) {
            log.debug("秒杀落单幂等跳过: userId={}, couponId={}, requestId={}", userId, couponId, requestId);
            asyncSeckillEnterService.markSuccess(requestId, "EXISTING");
            return;
        }

        String virtualId = null;
        boolean dbDeducted = false;
        try {
            ApiResponse<String> deductResponse = couponServiceFeignClient.deductDbSeckillStockOnly(couponId);
            virtualId = deductResponse != null ? deductResponse.getData() : null;
            int retry = 0;
            while ((virtualId == null || virtualId.isEmpty()) && retry < 3) {
                Backoff.linear(retry, 30L, 120L);
                if (Thread.currentThread().isInterrupted()) {
                    break;
                }
                deductResponse = couponServiceFeignClient.deductDbSeckillStockOnly(couponId);
                virtualId = deductResponse != null ? deductResponse.getData() : null;
                retry++;
            }
            if (virtualId == null || virtualId.isEmpty()) {
                log.error("异步落单 DB 扣库存失败，回补 Redis: requestId={}, couponId={}", requestId, couponId);
                asyncSeckillEnterService.compensateRedis(userId, couponId, requestId);
                stringRedisTemplate.delete(consumeLockKey);
                return;
            }
            dbDeducted = true;

            Coupon coupon = loadCouponForFulfill(couponId);
            if (coupon == null) {
                throw new BusinessException(ErrorCodes.COUPON_NOT_FOUND, "优惠券不存在");
            }
            if (!checkCouponCountLimit(userId, coupon.getType())) {
                throw new BusinessException(ErrorCodes.COUPON_LIMIT_EXCEEDED, "优惠券数量已达上限");
            }

            Order order = buildNewOrder(userId, couponId, virtualId, coupon);
            order.setRequestId(requestId);
            try {
                orderMapper.insert(order);
            } catch (Exception e) {
                if (isDuplicateEntryException(e)) {
                    log.warn("异步落单唯一约束命中，视为成功: userId={}, couponId={}", userId, couponId);
                    asyncSeckillEnterService.markSuccess(requestId, "DUP");
                    // DB 多扣了一份，回补分片
                    safeIncreaseSeckillStock(virtualId, couponId);
                    return;
                }
                throw e;
            }

            String userReceivedKey = USER_RECEIVED_KEY + userId + ":" + couponId;
            redisTemplate.opsForValue().set(userReceivedKey, true, 1, TimeUnit.HOURS);

            handlePostOrderCreation(userId, couponId, coupon.getType(), order, false);
            asyncSeckillEnterService.markSuccess(requestId, order.getId());

            try {
                OrderMessage resultMsg = new OrderMessage();
                resultMsg.setOrderId(order.getId());
                resultMsg.setUserId(userId);
                resultMsg.setCouponId(couponId);
                resultMsg.setVirtualId(virtualId);
                resultMsg.setStatus("SUCCESS");
                resultMsg.setCreateTime(new Date());
                kafkaTemplate.send(seckillOrderResultTopic, order.getId(), resultMsg);
            } catch (Exception e) {
                log.warn("发送 seckill_order_result 失败（订单已落库）: orderId={}", order.getId(), e);
            }

            log.debug("异步秒杀落单成功: orderId={}, requestId={}, userId={}, couponId={}",
                    order.getId(), requestId, userId, couponId);
        } catch (Exception e) {
            log.error("异步秒杀落单失败: requestId={}, userId={}, couponId={}", requestId, userId, couponId, e);
            if (dbDeducted) {
                safeIncreaseSeckillStock(virtualId, couponId);
            }
            asyncSeckillEnterService.compensateRedis(userId, couponId, requestId);
            // 释放消费锁，允许 Kafka 重试 / 用户重新入队后的合法消费
            try {
                stringRedisTemplate.delete(consumeLockKey);
            } catch (Exception ignore) {
                // ignore
            }
        }
    }

    /**
     * 落单前加载券元数据。开启 structured-concurrency 时用 JDK25 StructuredTaskScope（preview）封装，
     * 便于后续扩展并行只读准备；未开启或预览不可用则直调。
     */
    private Coupon loadCouponForFulfill(long couponId) {
        if (!structuredConcurrency) {
            return getCouponById(couponId);
        }
        try {
            return com.aliyun.seckill.couponkillorderservice.support.StructuredFulfillSupport
                    .loadCoupon(() -> getCouponById(couponId));
        } catch (Throwable t) {
            log.warn("Structured concurrency 加载券失败，回退直调: couponId={}, err={}", couponId, t.toString());
            return getCouponById(couponId);
        }
    }

    /**
     * 秒杀入口复用 createOrder（旧同步路径，保留给兼容调用）。
     * 冷却期由 Controller 在成功后统一设置。
     */
    @Override
    public boolean processSeckill(Long userId, Long couponId) {
        createOrder(userId, couponId);
        return true;
    }

    /**
     * 判断是否为数据库唯一性约束违反异常
     *
     * @param e 异常对象
     * @return 是否为唯一性约束违反异常
     */
    private boolean isDuplicateEntryException(Exception e) {
        if (e == null) {
            return false;
        }
        
        String message = e.getMessage();
        if (message == null) {
            return false;
        }
        
        // 检查常见的唯一性约束违反异常信息（兼容 PostgreSQL 23505 / duplicate key，以及历史 MySQL/SQLite 文案）
        return isDuplicateMessage(message) ||
               (e.getCause() != null &&
                e.getCause().getMessage() != null &&
                isDuplicateMessage(e.getCause().getMessage()));
    }

    private boolean isDuplicateMessage(String message) {
        if (message == null) {
            return false;
        }
        return message.contains("duplicate key value violates unique constraint") || // PostgreSQL
               message.contains("23505") ||                                          // PostgreSQL SQLState
               message.contains("Duplicate entry") ||                                // MySQL(历史)
               message.contains("UNIQUE constraint failed") ||
               message.contains("唯一约束违反") ||
               message.contains("唯一性约束");
    }
}