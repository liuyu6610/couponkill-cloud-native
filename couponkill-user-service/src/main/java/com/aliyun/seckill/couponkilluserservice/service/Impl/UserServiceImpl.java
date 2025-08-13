package com.aliyun.seckill.couponkilluserservice.service.Impl;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.couponkilluserservice.mapper.UserMapper;
import com.aliyun.seckill.couponkilluserservice.service.UserService;
import org.springframework.stereotype.Service;

@Service
public class UserServiceImpl implements UserService {

    private final UserMapper userMapper;

    public UserServiceImpl(UserMapper userMapper) {
        this.userMapper = userMapper;
    }

    @Override
    public User login(String username, String password) {
        return userMapper.selectByUsernameAndPassword(username, password);
    }

    @Override
    public User register(User user) {
        userMapper.insertUser(user);
        return user;
    }

    @Override
    public User getProfile(Long userId) {
        return userMapper.selectUserById(userId);
    }
}
