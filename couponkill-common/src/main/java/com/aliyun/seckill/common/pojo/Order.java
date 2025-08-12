// com.aliyun.seckill.common.pojo.Order.java
package com.aliyun.seckill.common.pojo;


import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data

public class Order implements Serializable {
    private static final long serialVersionUID = 1L;

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

    // 来源标识字段
    private Integer createdByJava = 0;
    private Integer createdByGo = 0;

    // 新增字段
    private String requestId;
    private Integer version; // 固定为0（Java端创建不影响Go端标识）
}