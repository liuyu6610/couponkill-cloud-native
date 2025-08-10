// com.aliyun.seckill.common.pojo.Order.java
package com.aliyun.seckill.common.pojo;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
@TableName("order")
public class Order implements Serializable {
    private static final long serialVersionUID = 1L;

    @TableId(type = IdType.INPUT)
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
    // 新增来源标识字段
    @TableField("created_by_java")
    private Integer createdByJava = 0; // Java端创建时设为1
    @TableField("created_by_go")
    private Integer createdByGo = 0; // 固定为0（Java端创建不影响Go端标识）
}