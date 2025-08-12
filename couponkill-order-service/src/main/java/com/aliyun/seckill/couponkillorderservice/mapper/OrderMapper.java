package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Order;
import org.apache.ibatis.annotations.*;

import java.util.List;

@Mapper
public interface OrderMapper {

    @Insert("INSERT INTO `order`(id, user_id, coupon_id, status, get_time, expire_time, use_time, cancel_time, create_time, update_time, created_by_java, created_by_go) " +
            "VALUES(#{order.id}, #{order.userId}, #{order.couponId}, #{order.status}, #{order.getTime}, #{order.expireTime}, " +
            "#{order.useTime}, #{order.cancelTime}, #{order.createTime}, #{order.updateTime}, #{order.createdByJava}, #{order.createdByGo})")
    int insert(@Param("order") Order order);

    @Select("SELECT * FROM `order` WHERE id = #{id}")
    Order selectById(@Param("id") String id);

    @Update("UPDATE `order` SET status = #{status}, cancel_time = #{cancelTime}, update_time = #{updateTime} WHERE id = #{id}")
    int updateStatus(@Param("id") String id, @Param("status") int status, @Param("cancelTime") java.time.LocalDateTime cancelTime,
                     @Param("updateTime") java.time.LocalDateTime updateTime);

    @Select("SELECT COUNT(*) FROM `order` WHERE user_id = #{userId} AND coupon_id = #{couponId} AND status IN (1, 2)")
    long countByUserAndCoupon(@Param("userId") Long userId, @Param("couponId") Long couponId);

    @Select("SELECT COUNT(*) FROM `order`")
    long countAll();

    @Select("SELECT * FROM `order` WHERE user_id = #{userId} LIMIT #{offset}, #{limit}")
    List<Order> selectByUserId(@Param("userId") Long userId, @Param("offset") int offset, @Param("limit") int limit);

    @Select("<script>" +
            "SELECT * FROM `order` " +
            "<where>" +
            "<if test='startTime != null and startTime != \"\"'>" +
            "AND create_time &gt;= #{startTime}" +
            "</if>" +
            "<if test='endTime != null and endTime != \"\"'>" +
            "AND create_time &lt;= #{endTime}" +
            "</if>" +
            "</where>" +
            "LIMIT #{offset}, #{limit}" +
            "</script>")
    List<Order> selectAllByCondition(@Param("startTime") String startTime, @Param("endTime") String endTime,
                                     @Param("offset") int offset, @Param("limit") int limit);
}
