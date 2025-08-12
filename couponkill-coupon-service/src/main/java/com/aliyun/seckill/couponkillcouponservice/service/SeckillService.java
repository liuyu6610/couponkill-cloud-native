package com.aliyun.seckill.couponkillcouponservice.service;

import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import lombok.RequiredArgsConstructor;
import org.apache.rocketmq.spring.core.RocketMQTemplate;
import org.springframework.core.io.ClassPathResource;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.messaging.support.MessageBuilder;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Collections;
import java.util.UUID;
@Service
@RequiredArgsConstructor
public class SeckillService {
    private final StringRedisTemplate redis;
    private final RocketMQTemplate rocketMQTemplate;

    private DefaultRedisScript<Long> enterScript;

    @jakarta.annotation.PostConstruct
    void init() throws Exception {
        var resource = new ClassPathResource("lua/enter_seckill.lua");
        var script = new String(resource.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        enterScript = new DefaultRedisScript<>(script, Long.class);
    }



    private String kStock(String couponId){ return "stock:"+couponId; }
    private String kCool(String couponId, String userId){ return "cd:"+couponId+":"+userId; }
    private String kDeduct(String reqId){ return "deduct:"+reqId; }
    private String kReq(String reqId){ return "req:"+reqId; }

    public record EnterResult(String requestId, String status, int err){}

    public EnterResult enter(String couponId, String userId, int cooldown, int deductTtl){
        String reqId = UUID.randomUUID().toString();
        Long r = redis.execute(enterScript,
                // KEYS
                Collections.unmodifiableList(java.util.List.of(kStock(couponId), kCool(couponId, userId), kDeduct(reqId))),
                // ARGV
                String.valueOf(cooldown), String.valueOf(deductTtl));
        if (r == null) return new EnterResult(reqId, "REJECTED", ErrorCodes.SYS_ERROR);
        if (r == 1L) {
            // 写请求状态为PENDING
            redis.opsForValue().set(kReq(reqId), "PENDING", Duration.ofMinutes(5));
            // 投递下游
            var cmd = new SeckillOrderCommand(reqId, couponId, userId, System.currentTimeMillis());
            rocketMQTemplate.send("seckill.order.create", MessageBuilder.withPayload(cmd).setHeader("reqId", reqId).build());
            return new EnterResult(reqId, "QUEUED", 0);
        }
        if (r == 0L) return new EnterResult(reqId, "REJECTED", ErrorCodes.OUT_OF_STOCK);
        if (r == -2L) return new EnterResult(reqId, "REJECTED", ErrorCodes.COOLING_DOWN);
        if (r == -3L) return new EnterResult(reqId, "QUEUED", 0); // 幂等重复点击，视为已入队
        return new EnterResult(reqId, "REJECTED", ErrorCodes.SYS_ERROR);
    }

    public String getResult(String requestId){ return redis.opsForValue().get(kReq(requestId)); }

    // 失败补偿：库存+1 & 发放常驻券标记（实际发券可交给另一个服务）
    public void compensateFail(String requestId, String couponId, String userId){
        redis.opsForValue().increment(kStock(couponId));
        // 可选：记录一张“常驻补偿券”的发放资格：coupon:comp:{userId} -> expire 1d
        redis.opsForValue().set("coupon:comp:"+userId, "1", Duration.ofDays(1));
        redis.opsForValue().set(kReq(requestId), "FAIL", Duration.ofMinutes(5));
    }

    public void markSuccess(String requestId, String orderId){
        redis.opsForValue().set(kReq(requestId), "SUCCESS:"+orderId, Duration.ofMinutes(5));
    }
}
