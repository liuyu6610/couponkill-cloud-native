package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.PostConstruct;
import java.time.Duration;
import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.List;
import java.util.Random;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

@Slf4j
@Service
public class CouponServiceImpl implements CouponService {

    @Autowired
    private CouponMapper couponMapper;

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;
    
    @Autowired
    private JdbcTemplate jdbcTemplate;

    private static final String COUPON_DETAIL_KEY = "coupon:detail:";
    private static final String COUPON_STOCK_KEY = "coupon:stock:";
    private static final String COUPON_AVAILABLE_KEY = "coupon:available";
    private static final int TOTAL_SHARDS = 64; // 4数据库 * 16表 = 64个分片

    @PostConstruct
    public void init() {
        // 应用启动时预热缓存
        asyncPreheatStockToRedis();
    }

    @Override
    public List<Coupon> getAvailableCoupons() {
        return couponMapper.selectAvailableCoupons();
    }

    @Override
    public List<Coupon> list() {
        return couponMapper.selectAll();
    }

    @Override
    public Coupon getCouponById(Long couponId) {
        // 先查缓存
        String key = COUPON_DETAIL_KEY + couponId;
        Object cached = redisTemplate.opsForValue().get(key);
        if (cached instanceof Coupon) {
            log.debug("从Redis缓存获取优惠券详情: couponId={}", couponId);
            return (Coupon) cached;
        }
        
        // 处理缓存穿透：如果缓存的是空值，直接返回null
        if (cached instanceof String && "".equals(cached)) {
            log.debug("Redis缓存为空值，避免查询数据库: couponId={}", couponId);
            return null;
        }

        // 缓存未命中，查数据库
        Coupon coupon = couponMapper.selectById(couponId);
        // 防止缓存穿透，对于不存在的数据也进行缓存，但设置较短的过期时间
        if (coupon == null) {
            redisTemplate.opsForValue().set(key, "", Duration.ofMinutes(5));
        } else {
            // 存入缓存，设置随机过期时间(30-40分钟)，避免缓存雪崩
            int expireMinutes = 30 + new Random().nextInt(11);
            redisTemplate.opsForValue().set(key, coupon, Duration.ofMinutes(expireMinutes));
            // 同时缓存库存信息
            redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId, coupon.getRemainingStock(), Duration.ofMinutes(expireMinutes));
        }
        return coupon;
    }

    @Override
    @Transactional
    public Coupon createCoupon(Coupon coupon) {
        // 设置创建和更新时间
        LocalDateTime now = LocalDateTime.now();
        if (coupon.getCreateTime() == null) {
            coupon.setCreateTime(now);
        }
        coupon.setUpdateTime(now);

        // 设置版本号
        if (coupon.getVersion() == null) {
            coupon.setVersion(0);
        }

        // 设置状态为有效
        if (coupon.getStatus() == null) {
            coupon.setStatus(1);
        }

        // 处理库存字段
        if (coupon.getType() != null && coupon.getType() == 2) {
            // 秒杀类型优惠券
            if (coupon.getSeckillTotalStock() == null) {
                coupon.setSeckillTotalStock(0);
            }
            if (coupon.getSeckillRemainingStock() == null) {
                coupon.setSeckillRemainingStock(coupon.getSeckillTotalStock());
            }
            // 普通库存设置为0
            coupon.setTotalStock(0);
            coupon.setRemainingStock(0);
        } else {
            // 普通优惠券
            if (coupon.getTotalStock() == null) {
                coupon.setTotalStock(0);
            }
            if (coupon.getRemainingStock() == null) {
                coupon.setRemainingStock(coupon.getTotalStock());
            }
            // 秒杀库存设置为0
            coupon.setSeckillTotalStock(0);
            coupon.setSeckillRemainingStock(0);
        }

        // 插入优惠券
        couponMapper.insertCoupon(coupon);
        
        // 清除缓存
        redisTemplate.delete(COUPON_DETAIL_KEY + coupon.getId());
        redisTemplate.delete(COUPON_AVAILABLE_KEY);
        redisTemplate.delete(COUPON_STOCK_KEY + coupon.getId());

        return coupon;
    }

    @Override
    public boolean grantCoupons(List<Long> userIds) {
        // 实现发放优惠券逻辑
        // 当前实现为简化版本，实际应根据业务需求实现
        return true;
    }

    @Override
    @Transactional
    public boolean deductStock(Long couponId) {
        try {
            // 先获取优惠券信息
            Coupon coupon = getCouponById(couponId);
            if (coupon == null) {
                return false;
            }

            // 使用乐观锁更新库存
            int rows = couponMapper.updateStock(couponId, -1, coupon.getVersion());
            if (rows > 0) {
                // 更新成功，同步更新Redis缓存
                String stockKey = COUPON_STOCK_KEY + couponId;
                redisTemplate.opsForValue().decrement(stockKey);

                // 异步更新优惠券详情缓存，避免阻塞主流程
                CompletableFuture.runAsync(() -> {
                    try {
                        Coupon updatedCoupon = couponMapper.selectById(couponId);
                        if (updatedCoupon != null) {
                            // 设置随机过期时间，避免缓存雪崩
                            int randomExpire = 30 + new Random().nextInt(10);
                            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, updatedCoupon,
                                    randomExpire, TimeUnit.MINUTES);
                        }
                    } catch (Exception e) {
                        log.warn("异步更新优惠券缓存失败，couponId: {}", couponId, e);
                    }
                });
                return true;
            }
            return false;
        } catch (Exception e) {
            log.error("扣减库存失败，couponId: {}", couponId, e);
            return false;
        }
    }

    @Override
    @Transactional
    public boolean increaseStock(Long couponId) {
        try {
            // 先获取优惠券信息
            Coupon coupon = getCouponById(couponId);
            if (coupon == null) {
                return false;
            }

            // 使用乐观锁更新库存
            int rows = couponMapper.updateStock(couponId, 1, coupon.getVersion());
            if (rows > 0) {
                // 更新成功，同步更新Redis缓存
                String stockKey = COUPON_STOCK_KEY + couponId;
                redisTemplate.opsForValue().increment(stockKey);

                // 异步更新优惠券详情缓存，避免阻塞主流程
                CompletableFuture.runAsync(() -> {
                    try {
                        Coupon updatedCoupon = couponMapper.selectById(couponId);
                        if (updatedCoupon != null) {
                            // 设置随机过期时间，避免缓存雪崩
                            int randomExpire = 30 + new Random().nextInt(10);
                            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, updatedCoupon,
                                    randomExpire, TimeUnit.MINUTES);
                        }
                    } catch (Exception e) {
                        log.warn("异步更新优惠券缓存失败，couponId: {}", couponId, e);
                    }
                });
                return true;
            }
            return false;
        } catch (Exception e) {
            log.error("增加库存失败，couponId: {}", couponId, e);
            return false;
        }
    }

    @Override
    public void updateStock(Long couponId, int newStock) {
        // 这个方法在当前接口中未实现，可以留空或者抛出异常
        throw new UnsupportedOperationException("updateStock method not implemented");
    }

    @Override
    public void handleExpiredCoupons() {
        // 当前实现为空，实际应根据业务需求实现
    }

    @Override
    public Integer deductStockWithShardIndex(Long couponId) {
        // 获取优惠券的所有分片
        List<Coupon> shards = getCouponShards(couponId);
        if (shards == null || shards.isEmpty()) {
            return null;
        }

        // 过滤出有库存的分片
        List<Coupon> availableShards = new ArrayList<>();
        for (Coupon shard : shards) {
            if (shard.getSeckillRemainingStock() > 0) {
                availableShards.add(shard);
            }
        }

        if (availableShards.isEmpty()) {
            return null; // 没有可用分片
        }

        // 随机选择一个分片
        Random random = new Random();
        Coupon selectedShard = availableShards.get(random.nextInt(availableShards.size()));

        // 扣减该分片的库存
        int version = selectedShard.getVersion() != null ? selectedShard.getVersion() : 0;
        int updated = couponMapper.updateStockByShardIndex(
            selectedShard.getId(), 
            selectedShard.getShardIndex(), 
            -1, // 扣减1个库存
            version);
            
        if (updated > 0) {
            // 更新缓存
            redisTemplate.delete(COUPON_DETAIL_KEY + couponId);
            String stockKey = COUPON_STOCK_KEY + couponId;
            Object currentStock = redisTemplate.opsForValue().get(stockKey);
            if (currentStock instanceof Integer) {
                redisTemplate.opsForValue().set(stockKey, (Integer) currentStock - 1, Duration.ofMinutes(30));
            }
            return selectedShard.getShardIndex();
        }

        return null;
    }

    @Override
    public String deductStockWithShardId(Long couponId) {
        Integer shardIndex = deductStockWithShardIndex(couponId);
        if (shardIndex != null) {
            return couponId + "_" + shardIndex;
        }
        return null;
    }

    @Override
    public List<Coupon> getCouponShards(Long id) {
        return couponMapper.selectByCouponId(id);
    }

    @Override
    public Coupon getRandomAvailableShard(Long couponId) {
        // 获取优惠券的所有分片
        List<Coupon> shards = getCouponShards(couponId);
        if (shards == null || shards.isEmpty()) {
            return null;
        }

        // 过滤出有库存的分片
        List<Coupon> availableShards = new ArrayList<>();
        for (Coupon shard : shards) {
            if (shard.getSeckillRemainingStock() > 0) {
                availableShards.add(shard);
            }
        }

        if (availableShards.isEmpty()) {
            return null; // 没有可用分片
        }

        // 随机选择一个分片
        Random random = new Random();
        return availableShards.get(random.nextInt(availableShards.size()));
    }
    
    @Override
    public void asyncPreheatStockToRedis() {
        try {
            // 预热所有主分片到Redis缓存
            List<Coupon> mainShards = couponMapper.selectMainShardsForCache();
            if (mainShards != null && !mainShards.isEmpty()) {
                redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
                    for (Coupon coupon : mainShards) {
                        String key = COUPON_DETAIL_KEY + coupon.getId();
                        // 设置随机过期时间(30-40分钟)，避免缓存雪崩
                        int expireMinutes = 30 + new Random().nextInt(11);
                        connection.set(key.getBytes(), serializeObject(coupon));
                        connection.expire(key.getBytes(), expireMinutes * 60);
                        
                        // 同时缓存库存信息
                        String stockKey = COUPON_STOCK_KEY + coupon.getId();
                        int stock = coupon.getType() == 2 ? coupon.getSeckillRemainingStock() : coupon.getRemainingStock();
                        connection.set(stockKey.getBytes(), String.valueOf(stock).getBytes());
                        connection.expire(stockKey.getBytes(), expireMinutes * 60);
                    }
                    return null;
                });
                log.info("预热优惠券缓存完成，共预热 {} 个优惠券", mainShards.size());
            }
        } catch (Exception e) {
            log.error("预热优惠券缓存失败", e);
        }
    }
    
    /**
     * 序列化对象为字节数组
     */
    private byte[] serializeObject(Object obj) {
        try {
            return redisTemplate.getStringSerializer().serialize(obj.toString());
        } catch (Exception e) {
            log.warn("序列化对象失败", e);
            return new byte[0];
        }
    }
}