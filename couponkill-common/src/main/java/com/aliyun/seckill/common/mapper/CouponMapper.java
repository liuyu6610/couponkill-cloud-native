// com.aliyun.seckill.coupon.mapper.CouponMapper.java
package com.aliyun.seckill.common.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Update;

@Mapper
public interface CouponMapper extends BaseMapper<Coupon> {
    @Update("update coupon set remaining_stock = remaining_stock + #{count} where id = #{couponId}")
    int updateStock(@Param("couponId") Long couponId, @Param("count") int count);
}