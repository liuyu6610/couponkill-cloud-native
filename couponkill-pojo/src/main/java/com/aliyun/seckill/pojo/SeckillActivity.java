// com.aliyun.seckill.pojo.SeckillActivity.java
package com.aliyun.seckill.pojo;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;
import java.time.LocalDateTime;

@Data
@TableName("seckill_activity")
public class SeckillActivity implements Serializable {
    private static final long serialVersionUID = 1L;

    @TableId(type = IdType.AUTO)
    private Long id;

    private Long couponId;

    private LocalDateTime startTime;

    private LocalDateTime endTime;

    /**
     * 状态(0-未开始,1-进行中,2-已结束)
     */
    private Integer status;

    private LocalDateTime createTime;

    private LocalDateTime updateTime;
}