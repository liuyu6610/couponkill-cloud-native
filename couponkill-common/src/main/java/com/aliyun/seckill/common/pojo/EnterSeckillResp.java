package com.aliyun.seckill.common.pojo;

import lombok.*;

/**
 * 秒杀热路径入队响应。status=QUEUED 表示 Redis 已预扣、订单异步创建中。
 */
@Getter
@Setter
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class EnterSeckillResp {
    /** QUEUED / REJECTED */
    private String status;
    private String requestId;
    /** 0=ok；与 ErrorCodes / ResultCode 对齐的业务码 */
    private Integer err;
    private String message;
}
