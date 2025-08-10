// com.aliyun.seckill.user.service.UserService.java
package com.aliyun.seckill.user.service;

import com.aliyun.seckill.common.pojo.User;
import com.baomidou.mybatisplus.extension.service.IService;

import java.util.Map;

public interface UserService extends IService<User> {
    /**
     * 用户注册
     */
    void register(String username, String password, String phone);

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
    void handleInactiveUsers ();
}