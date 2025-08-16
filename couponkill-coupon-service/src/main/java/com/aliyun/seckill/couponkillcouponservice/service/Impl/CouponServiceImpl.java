// com/aliyun/seckill/couponkillcouponservice/service/Impl/CouponServiceImpl.java
package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;
import java.util.concurrent.TimeUnit;

@Slf4j
@Service
public class CouponServiceImpl implements CouponService {

    @Autowired
    private CouponMapper couponMapper;

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

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
        couponMapper.insertCoupon(coupon);
        return coupon;
    }

    @Override
    public Coupon getCouponById(Long couponId) {
        // 先查缓存
        String key = COUPON_DETAIL_KEY + couponId;
        try {
            Object obj = redisTemplate.opsForValue().get(key);
            if (obj != null) {
                if (obj instanceof Coupon) {
                    Coupon coupon = (Coupon) obj;
                    // 验证关键字段是否存在
                    if (coupon.getId() != null && coupon.getName() != null) {
                        log.info("从缓存中获取优惠券信息：{}", coupon);
                        return coupon;
                    } else {
                        // 缓存中的对象字段不完整，删除并重新查询
                        log.warn("缓存中的优惠券数据不完整，清除缓存 key: {}", key);
                        redisTemplate.delete(key);
                    }
                } else {
                    // 如果缓存中的数据不是Coupon类型，删除错误的缓存
                    log.warn("缓存中的数据类型不正确，清除缓存 key: {}", key);
                    redisTemplate.delete(key);
                }
            }
        } catch (Exception e) {
            log.error("从缓存获取优惠券信息失败，couponId: {}", couponId, e);
            // 出现异常时忽略缓存，直接查询数据库
            // 同时清理可能存在问题的缓存
            try {
                redisTemplate.delete(key);
            } catch (Exception deleteException) {
                log.warn("清理异常缓存失败，couponId: {}", couponId, deleteException);
            }
        }

        // 缓存未命中或缓存数据有误，查数据库
        Coupon coupon = couponMapper.selectById(couponId);
        log.debug("从数据库中获取优惠券信息：{}", coupon);
        if (coupon != null) {
            try {
                // 设置缓存，添加随机过期时间防止缓存雪崩
                redisTemplate.opsForValue().set(key, coupon, 30 + (int)(Math.random() * 10), TimeUnit.MINUTES);
                // 缓存库存
                redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId, coupon.getRemainingStock());
            } catch (Exception e) {
                log.error("设置优惠券缓存失败，couponId: {}", couponId, e);
            }
        } else {
            try {
                // 缓存空对象，解决缓存穿透
                redisTemplate.opsForValue().set(key, new Coupon(), 5, TimeUnit.MINUTES);
            } catch (Exception e) {
                log.error("设置空优惠券缓存失败，couponId: {}", couponId, e);
            }
        }

        return coupon;
    }




    @Override
    @Transactional
    public boolean deductStock(Long couponId) {
        // 先尝试扣减Redis库存
        String stockKey = COUPON_STOCK_KEY + couponId;
        Long remain = redisTemplate.opsForValue().decrement(stockKey);

        if (remain == null || remain < 0) {
            // Redis扣减失败，回滚并查询数据库
            if (remain != null) {
                redisTemplate.opsForValue().increment(stockKey);
            }
            return false;
        }

        // 数据库扣减库存
        int rows = couponMapper.updateStock(couponId, -1);
        if (rows > 0) {
            // 更新缓存中的优惠券信息
            Coupon coupon = couponMapper.selectById(couponId);
            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon);
            return true;
        } else {
            // 数据库扣减失败，回滚Redis
            redisTemplate.opsForValue().increment(stockKey);
            return false;
        }
    }

    @Override
    @Transactional
    public boolean increaseStock(Long couponId) {
        // 数据库增加库存
        int rows = couponMapper.updateStock(couponId, 1);
        if (rows > 0) {
            // 更新Redis库存
            String stockKey = COUPON_STOCK_KEY + couponId;
            redisTemplate.opsForValue().increment(stockKey);

            // 更新缓存中的优惠券信息
            Coupon coupon = couponMapper.selectById(couponId);
            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon);
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

        int change = newStock - coupon.getRemainingStock();
        if (change != 0) {
            couponMapper.updateStock(couponId, change);

            // 更新缓存
            coupon.setRemainingStock(newStock);
            coupon.setUpdateTime(LocalDateTime.now());

            couponMapper.updateRemainingStock(couponId, newStock, LocalDateTime.now());

            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon);
            redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId, newStock);
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
