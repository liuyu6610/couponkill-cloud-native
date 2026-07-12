package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.common.pojo.SeckillOrderCommand;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import jakarta.annotation.PostConstruct;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.io.ClassPathResource;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/**
 * 秒杀热路径：Lua 预扣 Redis 库存 → Kafka 异步落单。
 * 关键路径禁止同步 Feign 扣 DB。
 */
@Service
@RequiredArgsConstructor
@Slf4j
public class AsyncSeckillEnterService {

    private final StringRedisTemplate stringRedisTemplate;
    private final KafkaTemplate<String, Object> kafkaTemplate;
    private final CouponServiceFeignClient couponServiceFeignClient;

    @Value("${kafka.topic.seckill-order-create:seckill_order_create}")
    private String seckillOrderCreateTopic;

    @Value("${couponkill.seckill.cooldown-seconds:2}")
    private int cooldownSeconds;

    @Value("${couponkill.seckill.deduct-ttl-seconds:300}")
    private int deductTtlSeconds;

    /** 热路径 Kafka send.get 有界等待（毫秒），超时走补偿 */
    @Value("${couponkill.seckill.kafka-send-timeout-ms:500}")
    private long kafkaSendTimeoutMs;

    private DefaultRedisScript<Long> enterScript;

    @PostConstruct
    void init() throws Exception {
        var resource = new ClassPathResource("lua/enter_seckill.lua");
        var script = new String(resource.getInputStream().readAllBytes(), StandardCharsets.UTF_8);
        enterScript = new DefaultRedisScript<>(script, Long.class);
    }

    private static String kStock(long couponId) {
        return "coupon:stock:" + couponId;
    }

    private static String kCool(long userId, long couponId) {
        return "seckill:cooldown:" + userId + ":" + couponId;
    }

    private static String kDeduct(long userId, long couponId) {
        return "seckill:deduct:" + userId + ":" + couponId;
    }

    private static String kReq(String requestId) {
        return "seckill:req:" + requestId;
    }

    public EnterSeckillResp enter(long userId, long couponId) {
        String requestId = UUID.randomUUID().toString();
        Long r = executeEnterLua(userId, couponId, requestId);

        // Redis 库存未预热：尝试一次 Feign 补救预热后重试 Lua
        if (r != null && r == -4L) {
            if (tryPreheatOnce(couponId)) {
                requestId = UUID.randomUUID().toString();
                r = executeEnterLua(userId, couponId, requestId);
            }
            if (r != null && r == -4L) {
                return rejected(requestId, ErrorCodes.NOT_PREHEATED, "秒杀库存未预热，请稍后重试");
            }
        }

        if (r == null) {
            return rejected(requestId, ErrorCodes.SYS_ERROR, "系统异常");
        }
        if (r == -3L) {
            String existingReq = stringRedisTemplate.opsForValue().get(kDeduct(userId, couponId));
            String rid = existingReq != null && !existingReq.isBlank() ? existingReq : requestId;
            return EnterSeckillResp.builder()
                    .status("QUEUED")
                    .requestId(rid)
                    .err(0)
                    .message("已受理，请勿重复点击")
                    .build();
        }
        if (r == -2L) {
            return rejected(requestId, ErrorCodes.COOLING_DOWN, "冷却中，请稍后再试");
        }
        if (r == 0L) {
            return rejected(requestId, ErrorCodes.OUT_OF_STOCK, "已抢完");
        }
        if (r != 1L) {
            return rejected(requestId, ErrorCodes.SYS_ERROR, "系统异常");
        }

        stringRedisTemplate.opsForValue().set(kReq(requestId), "PENDING", Duration.ofMinutes(10));

        SeckillOrderCommand cmd = SeckillOrderCommand.builder()
                .requestId(requestId)
                .couponId(String.valueOf(couponId))
                .userId(String.valueOf(userId))
                .ts(System.currentTimeMillis())
                .build();

        try {
            // 同步 ack + 有界超时：broker 正常时远小于 Feign+DB；超时/失败立即回补
            kafkaTemplate.send(seckillOrderCreateTopic, requestId, cmd)
                    .get(kafkaSendTimeoutMs, TimeUnit.MILLISECONDS);
        } catch (TimeoutException e) {
            log.error("秒杀入队超时({}ms)，回补 Redis: requestId={}, couponId={}, userId={}",
                    kafkaSendTimeoutMs, requestId, couponId, userId, e);
            compensateRedis(userId, couponId, requestId);
            return rejected(requestId, ErrorCodes.SYS_ERROR, "系统繁忙，请重试");
        } catch (Exception e) {
            log.error("秒杀入队失败，回补 Redis: requestId={}, couponId={}, userId={}",
                    requestId, couponId, userId, e);
            compensateRedis(userId, couponId, requestId);
            return rejected(requestId, ErrorCodes.SYS_ERROR, "系统繁忙，请重试");
        }

        return EnterSeckillResp.builder()
                .status("QUEUED")
                .requestId(requestId)
                .err(0)
                .message("秒杀受理中")
                .build();
    }

    private Long executeEnterLua(long userId, long couponId, String requestId) {
        return stringRedisTemplate.execute(
                enterScript,
                List.of(kStock(couponId), kCool(userId, couponId), kDeduct(userId, couponId)),
                String.valueOf(cooldownSeconds),
                String.valueOf(deductTtlSeconds),
                requestId
        );
    }

    private boolean tryPreheatOnce(long couponId) {
        try {
            ApiResponse<Boolean> resp = couponServiceFeignClient.preheatStock(couponId);
            boolean ok = resp != null && Boolean.TRUE.equals(resp.getData());
            if (!ok) {
                log.warn("秒杀补救预热未成功: couponId={}, resp={}", couponId, resp);
            }
            return ok;
        } catch (Exception e) {
            log.warn("秒杀补救预热调用失败: couponId={}", couponId, e);
            return false;
        }
    }

    public void compensateRedis(long userId, long couponId, String requestId) {
        try {
            stringRedisTemplate.opsForValue().increment(kStock(couponId));
            stringRedisTemplate.delete(kDeduct(userId, couponId));
            stringRedisTemplate.delete(kCool(userId, couponId));
            if (requestId != null) {
                stringRedisTemplate.opsForValue().set(kReq(requestId), "FAIL", Duration.ofMinutes(10));
            }
        } catch (Exception ex) {
            log.error("Redis 秒杀补偿失败，需人工对账: userId={}, couponId={}, requestId={}",
                    userId, couponId, requestId, ex);
        }
    }

    public void markSuccess(String requestId, String orderId) {
        stringRedisTemplate.opsForValue().set(kReq(requestId), "SUCCESS:" + orderId, Duration.ofMinutes(10));
    }

    public void markFail(String requestId) {
        stringRedisTemplate.opsForValue().set(kReq(requestId), "FAIL", Duration.ofMinutes(10));
    }

    public String getResult(String requestId) {
        return stringRedisTemplate.opsForValue().get(kReq(requestId));
    }

    private EnterSeckillResp rejected(String requestId, int err, String message) {
        return EnterSeckillResp.builder()
                .status("REJECTED")
                .requestId(requestId)
                .err(err)
                .message(message)
                .build();
    }
}
