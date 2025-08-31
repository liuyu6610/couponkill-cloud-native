// UserMapper.java
package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.User;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface UserMapper {
    User selectByUsername(String username);

    int insertUser(User user);

    Object selectById(Long userId);

    User selectByUsernameAndPassword(@Param("username") String username, @Param("password") String password);

    User selectUserById(@Param("id") Long id);

    // 查询失效用户
    List<User> selectInactiveUsers(@Param("inactiveThreshold") LocalDateTime inactiveThreshold);

    // 更新用户最后活跃时间
    int updateUserLastActiveTime(@Param("userId") Long userId, @Param("lastActiveTime") LocalDateTime lastActiveTime);
}