// com/aliyun/seckill/couponkillcouponservice/mapper/CouponMapper.java
package com.aliyun.seckill.couponkillcouponservice.mapper;

import com.aliyun.seckill.common.pojo.Coupon;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.time.LocalDateTime;
import java.util.List;

@Mapper
public interface CouponMapper {
    List<Coupon> selectAvailableCoupons();
    List<Coupon> selectAll();
    Coupon selectById(@Param("id") Long id);
    int insertCoupon(Coupon coupon);
    int batchGrantCoupons(@Param("userIds") List<Long> userIds);

    // 新增方法以支持库存更新
    int updateStock(@Param("couponId") Long couponId, @Param("change") int change);

    // 新增方法以直接更新剩余库存
    int updateRemainingStock(@Param("couponId") Long couponId,
                            @Param("remainingStock") Integer remainingStock,
                            @Param("updateTime") LocalDateTime updateTime);
    // 修改 updateStock 方法，添加字段名参数
    int updateStock(@Param("couponId") Long couponId, @Param("change") int change, @Param("fieldName") String fieldName);

}
