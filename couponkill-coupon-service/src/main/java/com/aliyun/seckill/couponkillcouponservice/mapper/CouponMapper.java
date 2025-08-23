// CouponMapper.java
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
    Coupon selectByVirtualId(@Param("virtualId") String virtualId);
    int insertCoupon(Coupon coupon);
    int batchGrantCoupons(@Param("userIds") List<Long> userIds);

    // 修改 updateStock 方法，添加版本号参数
    int updateStock(@Param("couponId") Long couponId,
                   @Param("change") int change,
                   @Param("version") int version);

    // 新增按虚拟ID更新库存的方法
    int updateStockByVirtualId(@Param("virtualId") String virtualId,
                              @Param("change") int change,
                              @Param("version") int version);

    // 修改 updateRemainingStock 方法，添加版本号参数
    int updateRemainingStock(@Param("couponId") Long couponId,
                            @Param("remainingStock") Integer remainingStock,
                            @Param("updateTime") LocalDateTime updateTime,
                            @Param("version") int version);

    // 新增按虚拟ID更新剩余库存的方法
    int updateRemainingStockByVirtualId(@Param("virtualId") String virtualId,
                                       @Param("remainingStock") Integer remainingStock,
                                       @Param("updateTime") LocalDateTime updateTime,
                                       @Param("version") int version);

    // 查询指定优惠券的所有虚拟分片
    List<Coupon> selectByCouponId(@Param("couponId") Long couponId);

    // 批量插入虚拟分片
    int batchInsertVirtualCoupons(@Param("coupons") List<Coupon> coupons);
}
