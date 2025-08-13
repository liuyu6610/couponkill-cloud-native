package com.aliyun.seckill.couponkillorderservice.mapper;

import com.aliyun.seckill.common.pojo.Order;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

@Mapper
public interface OrderMapper {
    int insertOrder(Order order);
    Order selectOrderById(@Param("id") Long id);
    int updateOrderStatus(@Param("orderId") Long orderId, @Param("status") String status);
}
