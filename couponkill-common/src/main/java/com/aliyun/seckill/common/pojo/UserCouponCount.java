// com.aliyun.seckill.common.pojo.UserCouponCount.java
package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data

public class UserCouponCount implements Serializable {
    private static final long serialVersionUID = 1L;


    private Long userId;
    private Integer totalCount;
    private Integer seckillCount;
    private Integer normalCount;
    private Integer expiredCount;
    private LocalDateTime updateTime;
    private Integer version;
}