// com.aliyun.seckill.coupon.service.impl.CouponServiceImpl.java
package com.aliyun.seckill.coupon.service.impl;

import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.result.ResultCode;
import com.aliyun.seckill.pojo.Coupon;
import com.aliyun.seckill.coupon.mapper.CouponMapper;
import com.aliyun.seckill.coupon.service.CouponService;
import com.baomidou.mybatisplus.core.conditions.query.QueryWrapper;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import javax.annotation.Resource;
import java.util.List;

@Service
public class CouponServiceImpl extends ServiceImpl<CouponMapper, Coupon> implements CouponService {

    @Resource
    private RedisTemplate<String, Object> redisTemplate;

    private static final String COUPON_STOCK_KEY = "coupon:stock:";
    private static final String COUPON_DETAIL_KEY = "coupon:detail:";

    @Override
    public List<Coupon> getAvailableCoupons() {
        // 先查缓存
        List<Coupon> coupons = (List<Coupon>) redisTemplate.opsForValue().get("coupon:available");
        if (coupons != null && !coupons.isEmpty()) {
            return coupons;
        }

        // 缓存未命中，查数据库
        QueryWrapper<Coupon> queryWrapper = new QueryWrapper<>();
        queryWrapper.eq("status", 1) // 已上架
                .gt("remaining_stock", 0); // 有库存
        coupons = list(queryWrapper);

        // 存入缓存，设置过期时间防止缓存雪崩
        if (coupons != null && !coupons.isEmpty()) {
            redisTemplate.opsForValue().set("coupon:available", coupons, 30, java.util.concurrent.TimeUnit.MINUTES);
        } else {
            // 缓存空值，解决缓存穿透
            redisTemplate.opsForValue().set("coupon:available", coupons, 5, java.util.concurrent.TimeUnit.MINUTES);
        }

        return coupons;
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
        coupon = getById(couponId);
        if (coupon == null) {
            // 缓存空值，解决缓存穿透
            redisTemplate.opsForValue().set(key, new Coupon(), 5, java.util.concurrent.TimeUnit.MINUTES);
            throw new BusinessException( String.valueOf( ResultCode.COUPON_NOT_FOUND ) );
        }

        // 存入缓存
        redisTemplate.opsForValue().set(key, coupon, 30, java.util.concurrent.TimeUnit.MINUTES);
        return coupon;
    }

    @Override
    @Transactional
    public boolean deductStock(Long couponId) {
        // 先尝试从Redis扣减库存
        String stockKey = COUPON_STOCK_KEY + couponId;
        Long remain = redisTemplate.opsForValue().decrement(stockKey);

        if (remain != null && remain >= 0) {
            // Redis扣减成功，同步到数据库
            int rows = baseMapper.deductStock(couponId);
            return rows > 0;
        } else if (remain != null && remain < 0) {
            // 库存不足，回滚Redis操作
            redisTemplate.opsForValue().increment(stockKey);
            return false;
        }

        // Redis操作失败，直接操作数据库
        int rows = baseMapper.deductStock(couponId);
        if (rows > 0) {
            // 同步到Redis
            Coupon coupon = getById(couponId);
            redisTemplate.opsForValue().set(stockKey, coupon.getRemainingStock());
            return true;
        }

        return false;
    }

    @Override
    @Transactional
    public boolean increaseStock(Long couponId) {
        // 先操作数据库
        int rows = baseMapper.increaseStock(couponId);
        if (rows > 0) {
            // 同步到Redis
            String stockKey = COUPON_STOCK_KEY + couponId;
            redisTemplate.opsForValue().increment(stockKey);

            // 更新缓存中的优惠券信息
            Coupon coupon = getById(couponId);
            redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon, 30, java.util.concurrent.TimeUnit.MINUTES);

            return true;
        }
        return false;
    }

    @Override
    @Transactional
    public void updateStock(Long couponId, int newStock) {
        Coupon coupon = getById(couponId);
        if (coupon == null) {
            throw new BusinessException( String.valueOf( ResultCode.COUPON_NOT_FOUND ) );
        }

        int diff = newStock - coupon.getRemainingStock();
        coupon.setRemainingStock(newStock);
        coupon.setTotalStock(newStock); // 重置总库存
        updateById(coupon);

        // 同步到Redis
        String stockKey = COUPON_STOCK_KEY + couponId;
        redisTemplate.opsForValue().set(stockKey, newStock);

        // 更新缓存中的优惠券信息
        redisTemplate.opsForValue().set(COUPON_DETAIL_KEY + couponId, coupon, 30, java.util.concurrent.TimeUnit.MINUTES);
    }
}