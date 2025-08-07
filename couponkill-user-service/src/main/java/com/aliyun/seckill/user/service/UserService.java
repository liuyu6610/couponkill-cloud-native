package com.aliyun.seckill.user.service;
import com.aliyun.seckill.pojo.User;
import com.baomidou.mybatisplus.extension.service.IService;

public interface UserService extends IService<User> {
    /**
     * 用户注册
     */
    void register(String username, String password, String phone, String email);

    /**
     * 用户登录
     */
    String login(String username, String password);

    /**
     * 根据ID获取用户信息
     */
    User getUserById(Long userId);
}
