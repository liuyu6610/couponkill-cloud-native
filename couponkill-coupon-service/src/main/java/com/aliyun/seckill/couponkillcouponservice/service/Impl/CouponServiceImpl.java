package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.serializer.Jackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.RedisSerializer;
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

    // 添加序列化器
    private final RedisSerializer<Object> serializer = new Jackson2JsonRedisSerializer<>(Object.class);

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

        // 缓存未命中，查数据库（查询所有分片并合并）
        List<Coupon> coupons = couponMapper.selectByCouponId(couponId);
        Coupon coupon = null;
        if (coupons != null && !coupons.isEmpty()) {
            // 合并所有分片的数据
            coupon = mergeCouponShards(coupons);
        }
        
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

    /**
     * 合并优惠券分片数据
     * @param shards 所有分片数据
     * @return 合并后的优惠券对象
     */
    private Coupon mergeCouponShards(List<Coupon> shards) {
        if (shards == null || shards.isEmpty()) {
            return null;
        }
        
        // 以第一个分片为基础
        Coupon merged = shards.get(0);
        
        // 合并库存信息
        int totalStock = 0;
        int remainingStock = 0;
        int seckillTotalStock = 0;
        int seckillRemainingStock = 0;
        
        for (Coupon shard : shards) {
            totalStock += shard.getTotalStock() != null ? shard.getTotalStock() : 0;
            remainingStock += shard.getRemainingStock() != null ? shard.getRemainingStock() : 0;
            seckillTotalStock += shard.getSeckillTotalStock() != null ? shard.getSeckillTotalStock() : 0;
            seckillRemainingStock += shard.getSeckillRemainingStock() != null ? shard.getSeckillRemainingStock() : 0;
        }
        
        merged.setTotalStock(totalStock);
        merged.setRemainingStock(remainingStock);
        merged.setSeckillTotalStock(seckillTotalStock);
        merged.setSeckillRemainingStock(seckillRemainingStock);
        
        return merged;
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
                        Coupon updatedCoupon = getCouponById(couponId);
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
                        Coupon updatedCoupon = getCouponById(couponId);
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
        try {
            // 使用更高效的分片选择策略，避免每次都查询所有分片
            // 先从Redis缓存中获取分片信息
            String shardsCacheKey = "coupon:shards:" + couponId;
            List<Coupon> shards = getCachedCouponShards(shardsCacheKey, couponId);
            
            if (shards == null || shards.isEmpty()) {
                log.warn("未找到优惠券 {} 的分片", couponId);
                return null;
            }

            // 筛选出有库存的分片
            List<Coupon> availableShards = new ArrayList<>();
            for (Coupon shard : shards) {
                if (shard.getSeckillRemainingStock() > 0) {
                    availableShards.add(shard);
                }
            }

            if (availableShards.isEmpty()) {
                log.warn("优惠券 {} 的所有分片都没有库存", couponId);
                // 更新缓存，标记无库存状态
                redisTemplate.opsForValue().set("coupon:out_of_stock:" + couponId, true, Duration.ofMinutes(1));
                return null;
            }

            // 使用轮询策略选择分片，避免热点问题
            String lastUsedShardKey = "coupon:last_shard:" + couponId;
            Integer lastShardIndex = (Integer) redisTemplate.opsForValue().get(lastUsedShardKey);
            if (lastShardIndex == null) {
                lastShardIndex = -1;
            }

            Coupon selectedShard = null;
            int startIndex = (lastShardIndex + 1) % availableShards.size();
            for (int i = 0; i < availableShards.size(); i++) {
                int index = (startIndex + i) % availableShards.size();
                Coupon shard = availableShards.get(index);
                if (shard.getSeckillRemainingStock() > 0) {
                    selectedShard = shard;
                    // 更新最后使用的分片索引
                    redisTemplate.opsForValue().set(lastUsedShardKey, index, 60, TimeUnit.SECONDS);
                    break;
                }
            }

            // 如果轮询没有找到合适的分片，则随机选择一个
            if (selectedShard == null) {
                Random random = new Random();
                selectedShard = availableShards.get(random.nextInt(availableShards.size()));
            }

            // 使用乐观锁更新库存
            int rows = couponMapper.updateStockByShardIndex(
                    selectedShard.getId(),
                    selectedShard.getShardIndex(),
                    -1,
                    selectedShard.getVersion() != null ? selectedShard.getVersion() : 0
            );

            if (rows > 0) {
                // 异步更新Redis缓存，避免阻塞主流程
                final Long couponIdFinal = couponId;
                CompletableFuture.runAsync(() -> {
                    try {
                        String stockKey = COUPON_STOCK_KEY + couponIdFinal;
                        redisTemplate.opsForValue().decrement(stockKey);
                        log.info("异步更新Redis库存成功: key={}", stockKey);
                        
                        // 更新分片缓存
                        String cacheKey = "coupon:shards:" + couponIdFinal;
                        List<Coupon> updatedShards = couponMapper.selectByCouponId(couponIdFinal);
                        redisTemplate.opsForValue().set(cacheKey, updatedShards, Duration.ofMinutes(5));
                    } catch (Exception e) {
                        log.warn("异步更新Redis缓存失败", e);
                    }
                });
                
                // 返回选中的分片索引
                log.info("成功扣减优惠券分片 {} 的库存", selectedShard.getShardIndex());
                return selectedShard.getShardIndex();
            }
            
            log.warn("更新优惠券 {} 分片 {} 库存失败", couponId, selectedShard.getShardIndex());
            return null;
        } catch (Exception e) {
            log.error("扣减库存并获取分片索引失败，couponId: {}", couponId, e);
            return null;
        }
    }

    /**
     * 从缓存获取优惠券分片信息，如果缓存未命中则查询数据库
     */
    private List<Coupon> getCachedCouponShards(String cacheKey, Long couponId) {
        try {
            Object cached = redisTemplate.opsForValue().get(cacheKey);
            if (cached instanceof List) {
                log.debug("从Redis缓存获取优惠券分片信息: couponId={}", couponId);
                return (List<Coupon>) cached;
            }
            
            // 缓存未命中，查询数据库
            List<Coupon> shards = couponMapper.selectByCouponId(couponId);
            if (shards != null && !shards.isEmpty()) {
                // 存入缓存，设置5分钟过期时间
                redisTemplate.opsForValue().set(cacheKey, shards, Duration.ofMinutes(5));
            }
            return shards;
        } catch (Exception e) {
            log.warn("获取优惠券分片信息失败，直接查询数据库: couponId={}", couponId, e);
            return couponMapper.selectByCouponId(couponId);
        }
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
            // 使用Jackson2JsonRedisSerializer进行序列化
            return serializer.serialize(obj);
        } catch (Exception e) {
            log.warn("序列化对象失败", e);
            return new byte[0];
        }
    }
}