package com.aliyun.seckill.couponkillorderservice.controller;

import com.aliyun.seckill.common.api.ApiResponse;
import com.aliyun.seckill.common.context.UserContext;
import com.aliyun.seckill.couponkillorderservice.domain.UserNotification;
import com.aliyun.seckill.couponkillorderservice.service.NotificationService;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Tag(name = "站内通知")
@RestController
@RequestMapping({"/order/notifications", "/api/v1/order/notifications"})
@RequiredArgsConstructor
public class NotificationController {

    private final NotificationService notificationService;

    @Operation(summary = "我的通知列表")
    @GetMapping("/mine")
    public ApiResponse<List<UserNotification>> mine(@RequestParam(defaultValue = "20") int limit) {
        Long userId = UserContext.requireCurrentUserId();
        return ApiResponse.success(notificationService.listMine(userId, limit));
    }

    @Operation(summary = "未读数量")
    @GetMapping("/unread-count")
    public ApiResponse<Map<String, Object>> unreadCount() {
        Long userId = UserContext.requireCurrentUserId();
        Map<String, Object> m = new LinkedHashMap<>();
        m.put("count", notificationService.unreadCount(userId));
        return ApiResponse.success(m);
    }

    @Operation(summary = "标记单条已读")
    @PostMapping("/{id}/read")
    public ApiResponse<Boolean> markRead(@PathVariable Long id) {
        Long userId = UserContext.requireCurrentUserId();
        return ApiResponse.success(notificationService.markRead(userId, id));
    }

    @Operation(summary = "全部标记已读")
    @PostMapping("/read-all")
    public ApiResponse<Integer> markAllRead() {
        Long userId = UserContext.requireCurrentUserId();
        return ApiResponse.success(notificationService.markAllRead(userId));
    }
}
