package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.exception.BusinessException;
import com.aliyun.seckill.common.pojo.Coupon;
import com.aliyun.seckill.couponkillorderservice.domain.ReservationStatuses;
import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import com.aliyun.seckill.couponkillorderservice.feign.CouponServiceFeignClient;
import com.aliyun.seckill.couponkillorderservice.mapper.SeckillReservationMapper;
import com.aliyun.seckill.couponkillorderservice.service.Impl.ReservationServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ReservationServiceImplTest {

    @Mock
    private SeckillReservationMapper reservationMapper;
    @Mock
    private CouponServiceFeignClient couponServiceFeignClient;

    @InjectMocks
    private ReservationServiceImpl reservationService;

    private Coupon seckillCoupon;

    @BeforeEach
    void setUp() {
        seckillCoupon = new Coupon();
        seckillCoupon.setId(1001L);
        seckillCoupon.setType(2);
        seckillCoupon.setStatus(1);
        seckillCoupon.setSeckillStartAt(LocalDateTime.now().plusHours(1));
        seckillCoupon.setSeckillEndAt(LocalDateTime.now().plusHours(2));
    }

    @Test
    void create_returnsExistingActiveReservation() {
        SeckillReservation existing = new SeckillReservation();
        existing.setId(9L);
        existing.setStatus(ReservationStatuses.PENDING);
        when(reservationMapper.selectActiveByUserAndCoupon(1L, 1001L)).thenReturn(existing);

        SeckillReservation got = reservationService.create(1L, 1001L);

        assertEquals(9L, got.getId());
        verify(couponServiceFeignClient, never()).getCouponById(any());
        verify(reservationMapper, never()).insert(any());
    }

    @Test
    void create_rejectsWhenAlreadyOnSale() {
        when(reservationMapper.selectActiveByUserAndCoupon(1L, 1001L)).thenReturn(null);
        seckillCoupon.setSeckillStartAt(LocalDateTime.now().minusMinutes(1));
        when(couponServiceFeignClient.getCouponById(1001L)).thenReturn(ApiResponse.success(seckillCoupon));

        BusinessException ex = assertThrows(BusinessException.class,
                () -> reservationService.create(1L, 1001L));
        assertTrue(ex.getMessage().contains("已开售"));
    }

    @Test
    void create_insertsPendingWithTriggerAt() {
        when(reservationMapper.selectActiveByUserAndCoupon(1L, 1001L)).thenReturn(null);
        when(couponServiceFeignClient.getCouponById(1001L)).thenReturn(ApiResponse.success(seckillCoupon));
        when(reservationMapper.insert(any())).thenAnswer(inv -> {
            SeckillReservation r = inv.getArgument(0);
            r.setId(42L);
            return 1;
        });
        SeckillReservation saved = new SeckillReservation();
        saved.setId(42L);
        saved.setStatus(ReservationStatuses.PENDING);
        saved.setTriggerAt(seckillCoupon.getSeckillStartAt());
        when(reservationMapper.selectById(42L)).thenReturn(saved);

        SeckillReservation got = reservationService.create(1L, 1001L);

        ArgumentCaptor<SeckillReservation> cap = ArgumentCaptor.forClass(SeckillReservation.class);
        verify(reservationMapper).insert(cap.capture());
        assertEquals(ReservationStatuses.PENDING, cap.getValue().getStatus());
        assertEquals(seckillCoupon.getSeckillStartAt(), cap.getValue().getTriggerAt());
        assertEquals(42L, got.getId());
    }

    @Test
    void cancel_rejectsNonPending() {
        SeckillReservation r = new SeckillReservation();
        r.setId(7L);
        r.setUserId(1L);
        r.setStatus(ReservationStatuses.FIRING);
        when(reservationMapper.selectById(7L)).thenReturn(r);

        BusinessException ex = assertThrows(BusinessException.class,
                () -> reservationService.cancel(1L, 7L));
        assertTrue(ex.getMessage().contains("PENDING"));
    }

    @Test
    void onFulfillResult_marksSuccess() {
        when(reservationMapper.markSuccessByUserCoupon(eq(1L), eq(1001L), eq("ord-1"))).thenReturn(1);
        reservationService.onFulfillResult(1L, 1001L, "ord-1", true);
        verify(reservationMapper).markSuccessByUserCoupon(1L, 1001L, "ord-1");
    }
}
