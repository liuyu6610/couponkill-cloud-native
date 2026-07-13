package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.api.ErrorCodes;
import com.aliyun.seckill.common.pojo.EnterSeckillResp;
import com.aliyun.seckill.couponkillorderservice.domain.ReservationStatuses;
import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.SeckillReservationMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Duration;
import java.time.LocalDateTime;
import java.util.List;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ReservationTriggerServiceTest {

    @Mock
    private SeckillReservationMapper reservationMapper;
    @Mock
    private OrderService orderService;
    @Mock
    private CouponServiceFeignClient couponServiceFeignClient;
    @Mock
    private AsyncSeckillEnterService asyncSeckillEnterService;
    @Mock
    private StringRedisTemplate stringRedisTemplate;
    @Mock
    private ValueOperations<String, String> valueOperations;

    @InjectMocks
    private ReservationTriggerService triggerService;

    @BeforeEach
    void setUp() {
        ReflectionTestUtils.setField(triggerService, "maxRetry", 3);
        ReflectionTestUtils.setField(triggerService, "batchSize", 50);
        when(stringRedisTemplate.opsForValue()).thenReturn(valueOperations);
    }

    @Test
    void fireDue_marksQueuedOnEnterSuccess() {
        SeckillReservation r = dueReservation();
        when(reservationMapper.selectDuePending(any(LocalDateTime.class), eq(50))).thenReturn(List.of(r));
        when(valueOperations.setIfAbsent(anyString(), eq("1"), any(Duration.class))).thenReturn(true);
        when(couponServiceFeignClient.getCouponById(1001L)).thenReturn(null);
        when(reservationMapper.claimForFire(9L, 0)).thenReturn(1);
        when(orderService.enterSeckillAsync(1L, 1001L)).thenReturn(
                EnterSeckillResp.builder().status("QUEUED").requestId("req-1").err(0).build());

        triggerService.fireDueReservations();

        verify(reservationMapper).markQueued(9L, "req-1", 1);
        verify(reservationMapper, never()).markFailed(anyLong(), anyInt(), anyString(), anyInt());
    }

    @Test
    void fireDue_marksFailedOnOutOfStock() {
        SeckillReservation r = dueReservation();
        when(reservationMapper.selectDuePending(any(LocalDateTime.class), eq(50))).thenReturn(List.of(r));
        when(valueOperations.setIfAbsent(anyString(), eq("1"), any(Duration.class))).thenReturn(true);
        when(couponServiceFeignClient.getCouponById(1001L)).thenReturn(null);
        when(reservationMapper.claimForFire(9L, 0)).thenReturn(1);
        when(orderService.enterSeckillAsync(1L, 1001L)).thenReturn(
                EnterSeckillResp.builder()
                        .status("REJECTED")
                        .err(ErrorCodes.OUT_OF_STOCK)
                        .message("无库存")
                        .build());

        triggerService.fireDueReservations();

        verify(reservationMapper).markFailed(9L, ErrorCodes.OUT_OF_STOCK, "无库存", 1);
    }

    @Test
    void fireDue_skipsWhenRedisLockHeld() {
        SeckillReservation r = dueReservation();
        when(reservationMapper.selectDuePending(any(LocalDateTime.class), eq(50))).thenReturn(List.of(r));
        when(valueOperations.setIfAbsent(anyString(), eq("1"), any(Duration.class))).thenReturn(false);

        triggerService.fireDueReservations();

        verify(orderService, never()).enterSeckillAsync(anyLong(), anyLong());
        verify(reservationMapper, never()).claimForFire(anyLong(), anyInt());
    }

    private static SeckillReservation dueReservation() {
        SeckillReservation r = new SeckillReservation();
        r.setId(9L);
        r.setUserId(1L);
        r.setCouponId(1001L);
        r.setStatus(ReservationStatuses.PENDING);
        r.setVersion(0);
        r.setRetryCount(0);
        r.setTriggerAt(LocalDateTime.now().minusSeconds(1));
        return r;
    }
}
