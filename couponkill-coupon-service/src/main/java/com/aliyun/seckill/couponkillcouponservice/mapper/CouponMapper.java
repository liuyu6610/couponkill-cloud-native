// 文件路径: com/aliyun/seckill/couponkillcouponservice/mapper/CouponMapper.java
package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.*;

import java.util.List;

@Mapper
public interface CouponMapper {

    @Select("SELECT * FROM coupon WHERE status = 1 AND remaining_stock > 0")
    List<Coupon> selectAvailableCoupons();

    @Select("SELECT * FROM coupon WHERE id = #{id}")
    Coupon selectById(@Param("id") Long id);

    @Update("UPDATE coupon SET remaining_stock = remaining_stock + #{count} WHERE id = #{couponId}")
    int updateStock(@Param("couponId") Long couponId, @Param("count") int count);

    @Select("SELECT * FROM coupon")
    List<Coupon> selectAll();

    @Update("UPDATE coupon SET remaining_stock = #{newStock}, update_time = #{updateTime} WHERE id = #{couponId}")
    int updateRemainingStock(@Param("couponId") Long couponId, @Param("newStock") int newStock, @Param("updateTime") java.time.LocalDateTime updateTime);
}
