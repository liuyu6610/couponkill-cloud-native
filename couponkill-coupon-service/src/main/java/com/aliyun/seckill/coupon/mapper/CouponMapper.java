package com.aliyun.seckill.coupon.mapper;

import com.aliyun.seckill.pojo.Coupon;
import com.baomidou.mybatisplus.core.conditions.Wrapper;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import org.apache.ibatis.session.ResultHandler;

import java.io.Serializable;
import java.util.Collection;
import java.util.List;
import java.util.Map;

public class CouponMapper implements BaseMapper<Coupon> {
    @Override
    public int insert (Coupon entity) {
        return 0;
    }

    @Override
    public int deleteById (Serializable id) {
        return 0;
    }

    @Override
    public int deleteById (Coupon entity) {
        return 0;
    }

    @Override
    public int delete (Wrapper<Coupon> queryWrapper) {
        return 0;
    }

    @Override
    public int deleteBatchIds (Collection<?> idList) {
        return 0;
    }

    @Override
    public int updateById (Coupon entity) {
        return 0;
    }

    @Override
    public int update (Coupon entity, Wrapper<Coupon> updateWrapper) {
        return 0;
    }

    @Override
    public Coupon selectById (Serializable id) {
        return null;
    }

    @Override
    public List<Coupon> selectBatchIds (Collection<? extends Serializable> idList) {
        return List.of();
    }

    @Override
    public void selectBatchIds (Collection<? extends Serializable> idList, ResultHandler<Coupon> resultHandler) {

    }

    @Override
    public Long selectCount (Wrapper<Coupon> queryWrapper) {
        return 0L;
    }

    @Override
    public List<Coupon> selectList (Wrapper<Coupon> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (Wrapper<Coupon> queryWrapper, ResultHandler<Coupon> resultHandler) {

    }

    @Override
    public List<Coupon> selectList (IPage<Coupon> page, Wrapper<Coupon> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (IPage<Coupon> page, Wrapper<Coupon> queryWrapper, ResultHandler<Coupon> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (Wrapper<Coupon> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (Wrapper<Coupon> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<Coupon> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<Coupon> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public <E> List<E> selectObjs (Wrapper<Coupon> queryWrapper) {
        return List.of();
    }

    @Override
    public <E> void selectObjs (Wrapper<Coupon> queryWrapper, ResultHandler<E> resultHandler) {

    }

    public int deductStock (Long couponId) {
    }

    public int increaseStock (Long couponId) {
    }
}
