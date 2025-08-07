package com.aliyun.seckill.user.mapper;

import com.aliyun.seckill.pojo.User;
import com.baomidou.mybatisplus.core.conditions.Wrapper;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import org.apache.ibatis.session.ResultHandler;

import java.io.Serializable;
import java.util.Collection;
import java.util.List;
import java.util.Map;

public class UserMapper implements BaseMapper<User> {
    @Override
    public int insert (User entity) {
        return 0;
    }

    @Override
    public int deleteById (Serializable id) {
        return 0;
    }

    @Override
    public int deleteById (User entity) {
        return 0;
    }

    @Override
    public int delete (Wrapper<User> queryWrapper) {
        return 0;
    }

    @Override
    public int deleteBatchIds (Collection<?> idList) {
        return 0;
    }

    @Override
    public int updateById (User entity) {
        return 0;
    }

    @Override
    public int update (User entity, Wrapper<User> updateWrapper) {
        return 0;
    }

    @Override
    public User selectById (Serializable id) {
        return null;
    }

    @Override
    public List<User> selectBatchIds (Collection<? extends Serializable> idList) {
        return List.of();
    }

    @Override
    public void selectBatchIds (Collection<? extends Serializable> idList, ResultHandler<User> resultHandler) {

    }

    @Override
    public Long selectCount (Wrapper<User> queryWrapper) {
        return 0L;
    }

    @Override
    public List<User> selectList (Wrapper<User> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (Wrapper<User> queryWrapper, ResultHandler<User> resultHandler) {

    }

    @Override
    public List<User> selectList (IPage<User> page, Wrapper<User> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectList (IPage<User> page, Wrapper<User> queryWrapper, ResultHandler<User> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (Wrapper<User> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (Wrapper<User> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public List<Map<String, Object>> selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<User> queryWrapper) {
        return List.of();
    }

    @Override
    public void selectMaps (IPage<? extends Map<String, Object>> page, Wrapper<User> queryWrapper, ResultHandler<Map<String, Object>> resultHandler) {

    }

    @Override
    public <E> List<E> selectObjs (Wrapper<User> queryWrapper) {
        return List.of();
    }

    @Override
    public <E> void selectObjs (Wrapper<User> queryWrapper, ResultHandler<E> resultHandler) {

    }
}
