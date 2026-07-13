package com.aliyun.seckill.couponkillorderservice.service;

import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;

import java.util.List;

public interface ReservationService {

    SeckillReservation create(Long userId, Long couponId);

    boolean cancel(Long userId, Long reservationId);

    List<SeckillReservation> listMine(Long userId);

    SeckillReservation getById(Long userId, Long reservationId);

    /** 履约结果回写（Kafka SUCCESS/FAILED） */
    void onFulfillResult(Long userId, Long couponId, String orderId, boolean success);
}
