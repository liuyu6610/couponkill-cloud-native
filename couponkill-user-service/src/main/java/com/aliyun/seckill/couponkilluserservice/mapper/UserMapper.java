package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.*;

@Mapper
public interface UserMapper {
    User selectByUsernameAndPassword(@Param("username") String username, @Param("password") String password);

    @Insert("INSERT INTO user(username, password, phone, email, status, create_time, update_time) " +
            "VALUES(#{username}, #{password}, #{phone}, #{email}, #{status}, #{createTime}, #{updateTime})")
    @Options(useGeneratedKeys = true, keyProperty = "id", keyColumn = "id")
    int insertUser(User user);


    User selectUserById(@Param("id") Long id);

    @Select("SELECT id, username, password, phone, email, status, create_time as createTime, update_time as updateTime FROM user WHERE username = #{username}")
    User selectByUsername(String username);

    @Select("SELECT user_id as userId, total_count as totalCount, seckill_count as seckillCount, " +
            "normal_count as normalCount, expired_count as expiredCount, update_time as updateTime " +
            "FROM user_coupon_count WHERE user_id = #{userId}")
    UserCouponCount selectById(@Param("userId") Long userId);

    @Insert("INSERT INTO user_coupon_count(user_id, total_count, seckill_count, normal_count, expired_count) " +
            "VALUES(#{count.userId}, #{count.totalCount}, #{count.seckillCount}, #{count.normalCount}, #{count.expiredCount})")
    int insert(@Param("count") UserCouponCount count);

    @Update("UPDATE user_coupon_count SET total_count = #{totalCount}, seckill_count = #{seckillCount}, " +
            "normal_count = #{normalCount}, expired_count = #{expiredCount}, update_time = NOW() WHERE user_id = #{userId}")
    int update(@Param("userId") Long userId, @Param("totalCount") int totalCount,
               @Param("seckillCount") int seckillCount, @Param("normalCount") int normalCount,
               @Param("expiredCount") int expiredCount);
}
