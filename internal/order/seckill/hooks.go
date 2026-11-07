package seckill

import (
	"context"
	"fmt"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// ResultEvent 描述一次秒杀请求的异步终态。
type ResultEvent struct {
	// RequestID 是秒杀请求号。
	RequestID string
	// UserID 是抢券用户。
	UserID int64
	// CouponID 是被抢的券。
	CouponID int64
	// Success 为 true 表示订单已落库（OrderID 有效），否则失败（Reason 有效）。
	Success bool
	// OrderID 是成功时的订单号。
	OrderID int64
	// Reason 是失败原因。
	Reason events.Reason
}

// ResultHook 在秒杀请求到达异步终态后被调用（消费者落单/失败、sweeper 收尾），例如预约模块借此回写结果、发通知。
//
// 约定：只在本次调用真正完成了终态迁移时调用一次（重放、被他人抢先终态的情况不会再调）；
// 尽力而为——返回的错误与 panic 只会被记录，绝不会让消息处理失败；因此实现方还需要有自己的对账兜底。
type ResultHook interface {
	OnSeckillResult(ctx context.Context, ev ResultEvent) error
}

// notify 依次调用所有钩子，隔离错误与 panic。
func (s *Service) notify(ctx context.Context, ev ResultEvent) {
	for _, h := range s.hooks {
		s.callHook(ctx, h, ev)
	}
}

func (s *Service) callHook(ctx context.Context, h ResultHook, ev ResultEvent) {
	defer func() {
		if r := recover(); r != nil {
			logx.From(ctx).Error("seckill result hook panicked", "request_id", ev.RequestID, "panic", fmt.Sprint(r))
		}
	}()
	if err := h.OnSeckillResult(ctx, ev); err != nil {
		logx.From(ctx).Warn("seckill result hook failed", "request_id", ev.RequestID, "err", err)
	}
}
