package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Orders;
import org.apache.ibatis.annotations.*;

import java.util.List;

@Mapper
public interface OrdersMapper {
    @Insert("INSERT INTO orders(coupon_id, created_at, request_id, status, user_id) " +
            "VALUES(#{couponId}, #{createdAt}, #{requestId}, #{status}, #{userId})")
    int insert(Orders order);

    @Select("SELECT * FROM orders WHERE request_id = #{requestId}")
    Orders selectByRequestId(@Param("requestId") String requestId);

    @Select("SELECT * FROM orders WHERE status = #{status}")
    List<Orders> selectByStatus(@Param("status") String status);

    @Update("UPDATE orders SET status = #{status} WHERE request_id = #{requestId}")
    int update(Orders order);
}
