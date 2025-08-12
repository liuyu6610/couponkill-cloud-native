// 文件路径: com/aliyun/seckill/couponkillcouponservice/service/Impl/CouponServiceImpl.java
package com.aliyun.seckill.couponkillcouponservice.service.Impl;

import com.aliyun.seckill.common.enums.ResultCode;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillcouponservice.mapper.CouponMapper;
import com.aliyun.seckill.couponkillcouponservice.service.CouponService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;
import java.util.concurrent.TimeUnit;

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
    public Coupon getCouponById(Long couponId) {
        // 先查缓存
        String key = COUPON_DETAIL_KEY + couponId;
        Coupon coupon = (Coupon) redisTemplate.opsForValue().get(key);
        if (coupon != null) {
            return coupon;
        }

        // 缓存未命中，查数据库
        coupon = couponMapper.selectById(couponId);
        if (coupon != null) {
            // 设置缓存，添加随机过期时间防止缓存雪崩
            redisTemplate.opsForValue().set(key, coupon, 30 + (int)(Math.random() * 10), TimeUnit.MINUTES);
            // 缓存库存
            redisTemplate.opsForValue().set(COUPON_STOCK_KEY + couponId, coupon.getRemainingStock());
        } else {
            // 缓存空对象，解决缓存穿透
            redisTemplate.opsForValue().set(key, new Coupon(), 5, TimeUnit.MINUTES);
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
    public void handleExpiredCoupons() {
        // 实现处理过期优惠券的逻辑
    }
}
