// com.aliyun.seckill.user.mapper.UserMapper.java
package com.aliyun.seckill.user.mapper;

import com.aliyun.seckill.common.pojo.User;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Select;

@Mapper
public interface UserMapper extends BaseMapper<User> {
    @Select("select * from user where username = #{username}")
    User selectByUsername(String username);
}