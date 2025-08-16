// com.aliyun.seckill.common.pojo.UserCouponCount.java
package com.aliyun.seckill.common.pojo;

import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
public class UserCouponCount implements Serializable {
    private static final long serialVersionUID = 1L;

    private Long userId; // 关联用户ID（对应表中user_id，主键）

    private Integer totalCount = 0;

    private Integer seckillCount = 0;

    private Integer normalCount = 0;

    private Integer expiredCount = 0;

    private LocalDateTime updateTime;

    private Integer version = 0;
}
