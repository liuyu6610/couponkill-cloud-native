package com.aliyun.seckill.common.pojo;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Date;

@Data
@AllArgsConstructor
@NoArgsConstructor
public class OrderMessage {
    private String orderId;
    private Long userId;
    private Long couponId;
    private Date createTime;
    private String status;
    private String virtualId;
}
