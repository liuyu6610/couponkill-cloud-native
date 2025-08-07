// com.aliyun.seckill.pojo.Order.java
package com.aliyun.seckill.pojo;

import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
@TableName("order")
public class Order implements Serializable {
    private static final long serialVersionUID = 1L;

    @TableId
    private String id;

    private Long userId;

    private Long couponId;

    /**
     * 状态(1-已创建,2-已使用,3-已过期,4-已取消)
     */
    private Integer status;

    private LocalDateTime getTime;

    private LocalDateTime expireTime;

    private LocalDateTime useTime;

    private LocalDateTime cancelTime;

    private LocalDateTime createTime;

    private LocalDateTime updateTime;
}