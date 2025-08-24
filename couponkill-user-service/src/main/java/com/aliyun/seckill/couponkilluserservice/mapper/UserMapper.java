// UserMapper.java
package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.User;
import org.apache.ibatis.annotations.*;

@Mapper
public interface UserMapper {

    // 根据用户名和密码查询（登录用）
    @Select("SELECT id, username, password, phone, email, status, create_time as createTime, " +
            "update_time as updateTime, last_active_time as lastActiveTime " +
            "FROM `user` WHERE username = #{username} AND password = #{password}")
    User selectByUsernameAndPassword(@Param("username") String username, @Param("password") String password);

    // 插入用户（注册用）- 修复后的版本
    @Insert("INSERT INTO `user`(id, username, password, phone, email, status, create_time, update_time) " +
            "VALUES(#{id}, #{username}, #{password}, #{phone}, #{email}, #{status}, #{createTime}, #{updateTime})")
    int insertUser(User user);

    // 根据ID查询用户
    @Select("SELECT id, username, password, phone, email, status, create_time as createTime, " +
            "update_time as updateTime, last_active_time as lastActiveTime " +
            "FROM `user` WHERE id = #{id}")
    User selectUserById(@Param("id") Long id);

    // 根据用户名查询（检查用户是否存在）
    @Select("SELECT id, username, password, phone, email, status, create_time as createTime, " +
            "update_time as updateTime, last_active_time as lastActiveTime " +
            "FROM `user` WHERE username = #{username}")
    User selectByUsername(String username);

    @Select("select * from `user` where id=#{userId}")
    Object selectById(Long userId);

    // 更新用户最后活跃时间
    @Update("UPDATE `user` SET last_active_time = NOW() WHERE id = #{id}")
    int updateLastActiveTime(@Param("id") Long id);
}
