// 文件路径: couponkill-coupon-service/src/main/java/com/aliyun/seckill/couponkillcouponservice/service/Impl/CouponServiceImpl.java
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
import java.util.concurrent.TimeUnit;

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

    @Override
    public List<Coupon> getAvailableCoupons() {
        return couponMapper.selectAvailableCoupons();
    }

    @Override
    public List<Coupon> list(){
        return couponMapper.selectAll();
    }

    @Override
    public Coupon createCoupon(Coupon coupon) {
        coupon.setCreateTime(LocalDateTime.now());
        coupon.setUpdateTime(LocalDateTime.now());
        coupon.setVersion(0); // 初始化版本号
        couponMapper.insertCoupon(coupon);
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
        Coupon coupon = couponMapper.selectById(couponId);
        log.debug("从数据库中获取优惠券信息：{}", coupon);
        if (coupon != null) {
            try {
                // 设置缓存，添加随机过期时间防止缓存雪崩
                int randomExpire = 30 + new Random().nextInt(10); // 30-40分钟过期
                redisTemplate.opsForValue().set(key, coupon, randomExpire, java.util.concurrent.TimeUnit.MINUTES);
                // 缓存库存
                redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId,
                        coupon.getType() == 2 ? coupon.getSeckillRemainingStock() : coupon.getRemainingStock());
            } catch (Exception e) {
                log.error("设置优惠券缓存失败，couponId: {}", couponId, e);
            }
        } else {
            try {
                // 缓存空对象，解决缓存穿透，设置较短过期时间
                redisTemplate.opsForValue().set(key, new Coupon(), 5, java.util.concurrent.TimeUnit.MINUTES);
            } catch (Exception e) {
                log.error("设置空优惠券缓存失败，couponId: {}", couponId, e);
            }
        }
        return coupon;
    }

    @Override
    @Transactional
    public boolean deductStock(Long couponId) {
        try {
            // 先获取优惠券信息，判断类型
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
                java.util.concurrent.CompletableFuture.runAsync(() -> {
                    try {
                        Coupon updatedCoupon = couponMapper.selectById(couponId);
                        if (updatedCoupon != null) {
                            int randomExpire = 30 + new java.util.Random().nextInt(10);
                            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, updatedCoupon,
                                    randomExpire, java.util.concurrent.TimeUnit.MINUTES);
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
        Coupon coupon = getCouponById(couponId);
        if (coupon == null) {
            return false;
        }

        // 使用乐观锁更新库存
        int rows = couponMapper.updateStock(couponId, 1, coupon.getVersion());
        if (rows > 0) {
            // 更新Redis库存
            String stockKey = COUPON_STOCK_KEY + couponId;
            redisTemplate.opsForValue().increment(stockKey);

            // 更新缓存中的优惠券信息
            Coupon updatedCoupon = couponMapper.selectById(couponId);
            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, updatedCoupon);
            return true;
        }
        return false;
    }

    @Override
    @Transactional
    public void updateStock(Long couponId, int newStock) {
        Coupon coupon = couponMapper.selectById(couponId);
        if (coupon == null) {
            throw new BusinessException(ResultCode.COUPON_NOT_FOUND);
        }

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
        try {
            List<Coupon> coupons = couponMapper.selectAll();
            redisTemplate.executePipelined((RedisCallback<Object>) connection -> {
                for (Coupon coupon : coupons) {
                    String stockKey = COUPON_STOCK_KEY + coupon.getId();
                    String detailKey = COUPON_DETAIL_KEY + coupon.getId();

                    // 预热布隆过滤器
                    couponBloomFilter.put(String.valueOf(coupon.getId()));

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
        } catch (Exception e) {
            log.error("预热Redis缓存失败", e);
        }
    }

    @Override
    public boolean grantCoupons(List<Long> userIds) {
        // 实现批量发放优惠券逻辑
        return true;
    }

    @Override
    public void handleExpiredCoupons() {
        // 实现处理过期优惠券的逻辑
    }

    // 在 CouponServiceImpl.java 中添加实现
    @Override
    public boolean lockStock(Long couponId) {
        // 实现库存锁定逻辑
        // 可以使用Redis分布式锁或者其他机制
        return deductStock(couponId); // 简单实现，实际可根据需求调整
    }

    @Override
    public boolean confirmDeductStock(Long couponId) {
        // TCC Confirm阶段，通常为空实现或执行确认操作
        return true;
    }

    @Override
    public boolean releaseStock(Long couponId) {
        // TCC Cancel阶段，释放之前锁定的库存
        return increaseStock(couponId);
    }
}
