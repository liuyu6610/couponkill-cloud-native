// 创建包: com.aliyun.seckill.couponkilluserservice.service.user
package com.aliyun.seckill.couponkilluserservice.service;

import com.aliyun.seckill.common.pojo.User;

import java.util.Map;

public interface UserService {
    /**
     * 用户注册
     */
     User register(String username, String password, String phone);

    /**
     * 用户登录
     */
    Map<String, Object> login(String username, String password);

    /**
     * 根据ID获取用户信息
     */
    User getUserById(Long userId);

    /**
     * 定时任务，处理失效用户
     */
    void handleInactiveUsers();
}
