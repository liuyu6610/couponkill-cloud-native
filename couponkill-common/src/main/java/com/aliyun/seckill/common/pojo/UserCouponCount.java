// com.aliyun.seckill.common.pojo.UserCouponCount.java
package com.aliyun.seckill.common.pojo;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;

@Data
@TableName("user_coupon_count")
public class UserCouponCount implements Serializable {
    private static final long serialVersionUID = 1L;

    @TableId(type = IdType.AUTO)
    private Long id;

    private Long userId;

    private Integer totalCount;

    private Integer seckillCount;
}