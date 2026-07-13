package com.aliyun.seckill.couponkillorderservice.controller;

import com.aliyun.seckill.common.context.UserContext;
import com.aliyun.seckill.common.result.Result;
import com.aliyun.seckill.couponkillorderservice.domain.SeckillReservation;
import com.aliyun.seckill.couponkillorderservice.service.ReservationService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.Map;

@Tag(name = "秒杀预约帮抢")
@RestController
@RequestMapping("/order/reservations")
@RequiredArgsConstructor
public class ReservationController {

    private final ReservationService reservationService;

    @Operation(summary = "创建预约帮抢", description = "开售前预约；到点由调度器代发本站秒杀入队")
    @PostMapping
    public Result<SeckillReservation> create(@RequestBody Map<String, Object> body) {
        Long userId = UserContext.requireCurrentUserId();
        Object raw = body == null ? null : body.get("couponId");
        if (raw == null) {
            return Result.fail(400, "couponId 不能为空");
        }
        Long couponId = Long.valueOf(String.valueOf(raw));
        return Result.success(reservationService.create(userId, couponId));
    }

    @Operation(summary = "取消预约", description = "仅 PENDING 可取消")
    @DeleteMapping("/{id}")
    public Result<Boolean> cancel(@PathVariable Long id) {
        Long userId = UserContext.requireCurrentUserId();
        return Result.success(reservationService.cancel(userId, id));
    }

    @Operation(summary = "我的预约列表")
    @GetMapping("/mine")
    public Result<List<SeckillReservation>> mine() {
        Long userId = UserContext.requireCurrentUserId();
        return Result.success(reservationService.listMine(userId));
    }

    @Operation(summary = "预约详情")
    @GetMapping("/{id}")
    public Result<SeckillReservation> detail(@PathVariable Long id) {
        Long userId = UserContext.requireCurrentUserId();
        return Result.success(reservationService.getById(userId, id));
    }
}
