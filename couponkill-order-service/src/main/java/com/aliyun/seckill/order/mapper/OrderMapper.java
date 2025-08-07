package com.aliyun.seckill.order.mapper;

import com.aliyun.seckill.pojo.Order;
import com.baomidou.mybatisplus.core.conditions.Wrapper;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import org.apache.ibatis.session.ResultHandler;

import java.io.Serializable;
import java.util.Collection;
import java.util.List;
import java.util.Map;

public class OrderMapper implements BaseMapper<Order> {
    @Override
    public int insert (Order entity) {
        return 0;
    }

    @Override
    public int deleteById (Serializable id) {
        return 0;
    }

    @Override
    public int deleteById (Order entity) {
        return 0;
    }

    @Override
    public int delete (Wrapper<Order> queryWrapper) {
        return 0;
    }

    @Override
    public int deleteBatchIds (Collection<?> idList) {
        return 0;
    }

    @Override
    public int updateById (Order entity) {
        return 0;
    }

    @Override
    public int update (Order entity, Wrapper<Order> updateWrapper) {
        return 0;
    }

    @Override
    public Order selectById (Serializable id) {
        return null;
    }

    @Override
    public List<Order> selectBatchIds (Collection<? extends Serializable> idList) {
        return List.of();
    }

    @Override
    public void selectBatchIds (Collection<? extends Serializable> idList, ResultHandler<Order> resultHandler) {

    }

    @Override
    public Long selectCount (Wrapper<Order> queryWrapper) {
        return 0L;
    }

    @Override
    public List<Order> selectList (Wrapper<Order> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (Wrapper<Order> queryWrapper, ResultHandler<Order> resultHandler) {

    }

    @Override
    public List<Order> selectList (IPage<Order> page, Wrapper<Order> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (IPage<Order> page, Wrapper<Order> queryWrapper, ResultHandler<Order> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (Wrapper<Order> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (Wrapper<Order> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<Order> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<Order> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public <E> List<E> selectObjs (Wrapper<Order> queryWrapper) {
        return List.of();
    }

    @Override
    public <E> void selectObjs (Wrapper<Order> queryWrapper, ResultHandler<E> resultHandler) {

    }
}
