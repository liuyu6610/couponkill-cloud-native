package com.aliyun.seckill.couponkilluserservice.service;


import com.aliyun.seckill.common.pojo.User;

public interface UserService {
    User login(String username, String password);
    User register(User user);
    User getProfile(Long userId);
}
