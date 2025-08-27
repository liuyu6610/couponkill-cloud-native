package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import com.google.common.hash.BloomFilter;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.PostConstruct;
import java.time.LocalDateTime;
import java.util.List;
import java.util.Random;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.stream.Collectors;

@Slf4j
@Service
public class CouponServiceImpl implements CouponService {

    @Autowired
    private CouponMapper couponMapper;
    @Autowired
    private BloomFilter<String> couponBloomFilter;
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;
    @Autowired
    private JdbcTemplate jdbcTemplate;

    private static final String COUPON_DETAIL_KEY = "coupon:detail:";
    private static final String COUPON_STOCK_KEY = "coupon:stock:";
    private static final String COUPON_AVAILABLE_KEY = "coupon:available";
    private static final int TOTAL_SHARDS = 64; // 4数据库 * 16表 = 64个分片

    @Override
    public List<Coupon> getAvailableCoupons() {
        return couponMapper.selectAvailableCoupons();
    }

    @Override
    public List<Coupon> list(){
        return couponMapper.selectAll();
    }

    @Override
    @Transactional
    public Coupon createCoupon(Coupon coupon) {
        coupon.setCreateTime(LocalDateTime.now());
        coupon.setUpdateTime(LocalDateTime.now());
        coupon.setVersion(0);
        coupon.setStatus(1); // 默认有效

        // 创建64个分片
        for (int i = 0; i < TOTAL_SHARDS; i++) {
            Coupon shard = Coupon.createShard(coupon, i, TOTAL_SHARDS);
            couponMapper.insertCoupon(shard);
        }

        log.info("为优惠券 {} 创建 {} 个分片", coupon.getId(), TOTAL_SHARDS);
        return coupon;
    }

    @Override
    public Coupon getCouponById(Long couponId) {
        String key = COUPON_DETAIL_KEY + couponId;

        // 使用布隆过滤器检查优惠券ID是否存在
        if (!couponBloomFilter.mightContain(String.valueOf(couponId))) {
            log.debug("布隆过滤器判断优惠券不存在，couponId: {}", couponId);
            return null;
        }

        try {
            Object obj = redisTemplate.opsForValue().get(key);
            if (obj != null) {
                // 处理 LinkedHashMap 情况
                if (obj instanceof java.util.LinkedHashMap) {
                    log.warn("缓存数据为 LinkedHashMap 类型，尝试转换为 Coupon 对象，key: {}", key);
                    // 清除异常缓存
                    redisTemplate.delete(key);
                }
                // 增强类型检查，确保是 Coupon 类型且关键字段有效
                else if (obj instanceof Coupon) {
                    Coupon coupon = (Coupon) obj;
                    if (coupon.getId() != null) {
                        return coupon;
                    } else {
                        log.warn("缓存中的优惠券数据不完整，清除缓存 key: {}", key);
                    }
                } else {
                    log.warn("缓存数据类型不正确，期望 Coupon 类型但实际为 {}，清除缓存 key: {}",
                            obj.getClass().getSimpleName(), key);
                }
                // 清除异常缓存
                redisTemplate.delete(key);
            }
        } catch (Exception e) {
            log.error("从缓存获取优惠券信息失败，couponId: {}", couponId, e);
            try {
                redisTemplate.delete(key);
            } catch (Exception deleteException) {
                log.warn("清理异常缓存失败，couponId: {}", couponId, deleteException);
            }
        }

        // 缓存未命中或异常，查询数据库
        List<Coupon> shards = couponMapper.selectByCouponId(couponId);
        Coupon coupon = mergeShards(shards);
        log.debug("从数据库中获取优惠券信息：{}", coupon);
        if (coupon != null) {
            try {
                // 设置缓存，添加随机过期时间防止缓存雪崩
                int randomExpire = 30 + new Random().nextInt(10); // 30-40分钟过期
                redisTemplate.opsForValue().set(key, coupon, randomExpire, TimeUnit.MINUTES);
                // 缓存库存
                redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId,
                        coupon.getType() == 2 ? coupon.getSeckillRemainingStock() : coupon.getRemainingStock());
            } catch (Exception e) {
                log.error("设置优惠券缓存失败，couponId: {}", couponId, e);
            }
        } else {
            try {
                // 缓存空对象，解决缓存穿透，设置较短过期时间
                redisTemplate.opsForValue().set(key, new Coupon(), 5, TimeUnit.MINUTES);
            } catch (Exception e) {
                log.error("设置空优惠券缓存失败，couponId: {}", couponId, e);
            }
        }
        return coupon;
    }

    // 合并分片信息
    private Coupon mergeShards(List<Coupon> shards) {
        if (shards == null || shards.isEmpty()) {
            return null;
        }

        Coupon merged = new Coupon();
        Coupon first = shards.get(0);
        merged.setId(first.getId());
        merged.setName(first.getName());
        merged.setDescription(first.getDescription());
        merged.setType(first.getType());
        merged.setFaceValue(first.getFaceValue());
        merged.setMinSpend(first.getMinSpend());
        merged.setValidDays(first.getValidDays());
        merged.setPerUserLimit(first.getPerUserLimit());
        merged.setStatus(first.getStatus());
        merged.setCreateTime(first.getCreateTime());
        merged.setUpdateTime(first.getUpdateTime());

        int totalStock = 0;
        int seckillTotalStock = 0;
        int remainingStock = 0;
        int seckillRemainingStock = 0;

        for (Coupon shard : shards) {
            if (shard != null) {
                totalStock += shard.getTotalStock();
                seckillTotalStock += shard.getSeckillTotalStock();
                remainingStock += shard.getRemainingStock();
                seckillRemainingStock += shard.getSeckillRemainingStock();
            }
        }

        merged.setTotalStock(totalStock);
        merged.setSeckillTotalStock(seckillTotalStock);
        merged.setRemainingStock(remainingStock);
        merged.setSeckillRemainingStock(seckillRemainingStock);

        return merged;
    }

    @Override
    @Transactional
    public boolean deductStock(Long couponId) {
        try {
            Integer shardIndex = deductStockWithShardIndex(couponId);
            return shardIndex != null;
        } catch (Exception e) {
            log.error("扣减库存失败，couponId: {}", couponId, e);
            return false;
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
    public Integer deductStockWithShardIndex(Long couponId) {
        try {
            // 获取该优惠券的所有分片
            List<Coupon> shards = couponMapper.selectByCouponId(couponId);
            if (shards == null || shards.isEmpty()) {
                log.warn("未找到优惠券 {} 的分片", couponId);
                return null;
            }

            // 筛选出有库存的分片
            List<Coupon> availableShards = shards.stream()
                    .filter(c -> c.getSeckillRemainingStock() != null && c.getSeckillRemainingStock() > 0)
                    .collect(Collectors.toList());

            if (availableShards.isEmpty()) {
                log.warn("优惠券 {} 的所有分片都没有库存", couponId);
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
                // 检查库存是否足够
                if (shard.getSeckillRemainingStock() != null && shard.getSeckillRemainingStock() > 0) {
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
                final Long couponIdFinal = couponId; // 将变量设为final
                CompletableFuture.runAsync(() -> {
                    try {
                        String stockKey = COUPON_STOCK_KEY + couponIdFinal;
                        redisTemplate.opsForValue().decrement(stockKey);
                        log.info("异步更新Redis库存成功: key={}", stockKey);
                    } catch (Exception e) {
                        log.error("异步更新Redis库存失败: couponId={}", couponIdFinal, e);
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

    @Override
    public boolean increaseStock(Long couponId) {
        return false;
    }

    @Override
    public List<Coupon> getCouponShards(Long id) {
        return couponMapper.selectByCouponId(id);
    }

    /**
     * 随机获取一个有库存的优惠券分片
     * @param couponId 优惠券ID
     * @return 有库存的优惠券分片，如果没有则返回null
     */
    public Coupon getRandomAvailableShard(Long couponId) {
        List<Coupon> shards = couponMapper.selectByCouponId(couponId);
        if (shards == null || shards.isEmpty()) {
            return null;
        }

        // 筛选出有库存的分片
        List<Coupon> availableShards = shards.stream()
                .filter(c -> c.getSeckillRemainingStock() != null && c.getSeckillRemainingStock() > 0)
                .collect(Collectors.toList());

        if (availableShards.isEmpty()) {
            return null;
        }

        // 随机选择一个分片
        Random random = new Random();
        return availableShards.get(random.nextInt(availableShards.size()));
    }

    @Override
    @Transactional
    public void updateStock(Long couponId, int newStock) {
        List<Coupon> shards = couponMapper.selectByCouponId(couponId);
        if (shards == null || shards.isEmpty()) {
            throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
        }

        Coupon coupon = mergeShards(shards);
        int change = newStock - (coupon.getType() == 2 ? coupon.getSeckillRemainingStock() : coupon.getRemainingStock());
        if (change != 0) {
            // 使用乐观锁更新库存
            int rows = couponMapper.updateStock(couponId, change, coupon.getVersion());
            if (rows > 0) {
                // 更新缓存
                coupon.setRemainingStock(newStock);
                coupon.setUpdateTime(LocalDateTime.now());
                coupon.setVersion(coupon.getVersion() + 1);

                redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon);
                redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId, newStock);
            }
        }
    }

    @PostConstruct
    public void preheatStockToRedis() {
        // 异步预热缓存，避免阻塞应用启动
        CompletableFuture.runAsync(this::asyncPreheatStockToRedis, 
                Executors.newFixedThreadPool(10));
    }
    
    public void asyncPreheatStockToRedis() {
        try {
            // 查询所有主分片用于预热缓存
            List<Coupon> coupons = couponMapper.selectMainShardsForCache();
            log.info("预热缓存，共找到 {} 个优惠券主分片", coupons != null ? coupons.size() : 0);
            
            if (coupons != null && !coupons.isEmpty()) {
                redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
                    for (Coupon coupon : coupons) {
                        String stockKey = COUPON_STOCK_KEY + coupon.getId();
                        String detailKey = COUPON_DETAIL_KEY + coupon.getId();

                        // 预热布隆过滤器
                        couponBloomFilter.put(String.valueOf(coupon.getId()));
                        log.debug("预热布隆过滤器: couponId={}", coupon.getId());

                        // 设置库存缓存
                        if (coupon.getType() == 2) { // 秒杀类型
                            redisTemplate.opsForValue().set(stockKey, coupon.getSeckillRemainingStock());
                        } else { // 普通类型
                            redisTemplate.opsForValue().set(stockKey, coupon.getRemainingStock());
                        }

                        // 设置详情缓存，带随机过期时间
                        redisTemplate.opsForValue().set(detailKey, coupon,
                                30 + (int)(Math.random() * 10), TimeUnit.MINUTES);
                    }
                    return null;
                });
                log.info("预热完成，共加载 {} 个优惠券", coupons.size());
            } else {
                log.info("没有找到需要预热的优惠券");
            }
        } catch (Exception e) {
            log.error("预热Redis缓存失败", e);
        }
    }

    @Override
    public boolean grantCoupons(List<Long> userIds) {
        if (userIds == null || userIds.isEmpty()) {
            log.warn("用户ID列表为空，无法发放优惠券");
            return false;
        }

        try {
            // 实现批量发放优惠券逻辑
            // 这里可以调用用户服务接口或者消息队列来实现
            for (Long userId : userIds) {
                log.info("向用户 {} 发放优惠券", userId);
                // 实际业务逻辑需要根据具体需求实现
            }
            return true;
        } catch (Exception e) {
            log.error("批量发放优惠券失败，userIds: {}", userIds, e);
            return false;
        }
    }

    @Override
    public void handleExpiredCoupons() {
        try {
            // 实现处理过期优惠券的逻辑
            // 1. 查询已过期但状态仍为有效的优惠券
            LocalDateTime expireTime = LocalDateTime.now().minusDays(1); // 查找1天前创建的优惠券
            List<Coupon> expiredCoupons = couponMapper.selectExpiredCoupons(expireTime);

            if (expiredCoupons != null && !expiredCoupons.isEmpty()) {
                for (Coupon coupon : expiredCoupons) {
                    // 2. 更新优惠券状态为已过期
                    int result = couponMapper.updateCouponStatus(coupon.getId(), 0); // 0表示无效
                    if (result > 0) {
                        log.info("优惠券 {} 已过期，状态已更新", coupon.getId());

                        // 3. 清除相关缓存
                        redisTemplate.delete(COUPON_DETAIL_KEY + coupon.getId());
                        redisTemplate.delete(COUPON_STOCK_KEY + coupon.getId());
                    }
                }
            }
        } catch (Exception e) {
            log.error("处理过期优惠券失败", e);
        }
    }
}
