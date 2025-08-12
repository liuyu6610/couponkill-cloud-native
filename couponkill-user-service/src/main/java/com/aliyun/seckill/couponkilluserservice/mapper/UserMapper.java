// 文件路径: com/aliyun/seckill/couponkilluserservice/mapper/UserMapper.java
package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.User;
import org.apache.ibatis.annotations.*;

@Mapper
public interface UserMapper {

    @Select("SELECT * FROM user WHERE username = #{username}")
    User selectByUsername(@Param("username") String username);

    @Select("SELECT * FROM user WHERE id = #{id}")
    User selectById(@Param("id") Long id);

    @Insert("INSERT INTO user(username, password, phone, status, create_time, update_time) " +
            "VALUES(#{user.username}, #{user.password}, #{user.phone}, #{user.status}, #{user.createTime}, #{user.updateTime})")
    @Options(useGeneratedKeys = true, keyProperty = "user.id")
    int insert(@Param("user") User user);

    @Update("UPDATE user SET password = #{password}, update_time = #{updateTime} WHERE id = #{id}")
    int updatePassword(@Param("id") Long id, @Param("password") String password, @Param("updateTime") java.time.LocalDateTime updateTime);
}
