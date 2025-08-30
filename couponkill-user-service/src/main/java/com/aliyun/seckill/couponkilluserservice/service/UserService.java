// 创建包: com.aliyun.seckill.couponkilluserservice.service.user
package com.aliyun.seckill.couponkilluserservice.service;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.pojo.UserCouponCount;

import java.util.List;
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
    
    /**
     * 获取用户优惠券统计信息
     * @param userId 用户ID
     * @return 用户优惠券统计信息
     */
    UserCouponCount getUserCouponCount(Long userId);
    
    /**
     * 更新用户秒杀优惠券数量
     * @param userId 用户ID
     * @param count 变化数量（正数表示增加，负数表示减少）
     */
    void updateSeckillCouponCount(Long userId, int count);
    
    /**
     * 更新用户普通优惠券数量
     * @param userId 用户ID
     * @param count 变化数量（正数表示增加，负数表示减少）
     */
    void updateNormalCouponCount(Long userId, int count);
    
    /**
     * 批量插入用户
     * @param users 用户列表
     */
    void batchInsertUsers(List<User> users);
}