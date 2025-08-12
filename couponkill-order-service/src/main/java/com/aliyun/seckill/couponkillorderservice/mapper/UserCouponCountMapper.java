// 文件路径: com/aliyun/seckill/couponkillorderservice/mapper/UserCouponCountMapper.java
package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.UserCouponCount;
import org.apache.ibatis.annotations.*;

@Mapper
public interface UserCouponCountMapper {

    @Select("SELECT * FROM user_coupon_count WHERE user_id = #{userId}")
    UserCouponCount selectById(@Param("userId") Long userId);

    @Insert("INSERT INTO user_coupon_count(user_id, total_count, seckill_count) VALUES(#{count.userId}, #{count.totalCount}, #{count.seckillCount})")
    int insert(@Param("count") UserCouponCount count);

    @Update("UPDATE user_coupon_count SET total_count = #{totalCount}, seckill_count = #{seckillCount} WHERE user_id = #{userId}")
    int update(@Param("userId") Long userId, @Param("totalCount") int totalCount, @Param("seckillCount") int seckillCount);
}
