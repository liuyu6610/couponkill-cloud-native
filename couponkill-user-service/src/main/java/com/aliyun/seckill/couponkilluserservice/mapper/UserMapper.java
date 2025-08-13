package com.aliyun.seckill.couponkilluserservice.mapper;

import com.aliyun.seckill.common.pojo.User;
import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.*;

;

@Mapper
public interface UserMapper {
    User selectByUsernameAndPassword(@Param("username") String username, @Param("password") String password);
    int insertUser(User user);
    User selectUserById(@Param("id") Long id);
    @Select("SELECT user_id, total_count, seckill_count, normal_count, expired_count, update_time, version FROM user_coupon_count WHERE user_id = #{userId}")
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
