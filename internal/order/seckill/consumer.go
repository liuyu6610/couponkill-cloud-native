package seckill

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Consume 在消费组 cfg 上运行建单消费者，直到 ctx 取消（返回 nil）或出现不可恢复的错误。
// cfg.Group 应为 events.GroupSeckillOrderCreate，cfg.Topics 为 events.TopicSeckillOrderCreate。
func (s *Service) Consume(ctx context.Context, cfg kafkax.ConsumerConfig) error {
	if err := kafkax.Consume(ctx, cfg, s.Handle); err != nil {
		return fmt.Errorf("seckill consumer: %w", err)
	}
	return nil
}

// Handle 处理一条建单命令（设计 §9 的消费者）。它必须对重放幂等：
// 返回 nil 表示该请求已到达终态（或无需处理），消息可提交；返回错误表示可重试，且绝不会在此之前做补偿。
// 载荷非法返回 kafkax.Permanent 错误，直接进死信。
func (s *Service) Handle(ctx context.Context, msg kafkax.Message) error {
	cmd, err := events.UnmarshalSeckillOrderCreate(msg.Value)
	if err != nil {
		return kafkax.Permanent(err)
	}
	ctx = logx.With(ctx, "request_id", cmd.RequestID, "user_id", cmd.UserID, "coupon_id", cmd.CouponID)

	code, err := s.runAdvance(ctx, cmd.RequestID, cmd.UserID, cmd.CouponID, events.StatusProcessing, 0)
	if err != nil {
		return err
	}
	switch code {
	case advSuccessExists:
		return nil
	case advFailedExists:
		return s.reconcileFailed(ctx, cmd)
	}
	return s.fulfill(ctx, cmd)
}

// fulfill 是 Handle 在请求未到终态时的主体：恢复已落库的订单 → 限额 → 读券 → 扣库存 → 事务建单 → 置 SUCCESS。
func (s *Service) fulfill(ctx context.Context, cmd events.SeckillOrderCreate) error {
	// 崩溃在「订单已提交、尚未置 SUCCESS」之间时，重投直接按已落库的订单收尾，不再碰库存。
	existing, found, err := orders.FindByRequest(ctx, s.db, cmd.UserID, cmd.RequestID)
	if err != nil {
		return err
	}
	if found {
		return s.finalizeSuccess(ctx, cmd, existing.ID)
	}

	reason, err := s.checkLimits(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	if reason != "" {
		return s.fail(ctx, cmd, reason, false, false)
	}

	info, err := s.meta.Get(ctx, cmd.CouponID)
	if err != nil {
		if retryable(ctx, err) {
			return err
		}
		return s.fail(ctx, cmd, events.ReasonForError(err), true, false)
	}

	if _, err := s.coupon.DeductStock(ctx, internalapi.DeductRequest{RequestID: cmd.RequestID, CouponID: cmd.CouponID, UserID: cmd.UserID}); err != nil {
		switch {
		case retryable(ctx, err):
			// 超时/5xx 可能已经扣了：同一 requestId 重试安全，补偿交给 sweeper，这里绝不补偿。
			return err
		case errors.Is(err, apperr.OutOfStock):
			return s.fail(ctx, cmd, events.ReasonOutOfStock, false, false)
		default:
			return s.fail(ctx, cmd, events.ReasonForError(err), true, false)
		}
	}

	orderID, err := s.persist(ctx, cmd, info)
	switch {
	case err == nil:
	case dbx.IsUniqueViolation(err, ""):
		// 吸收了同一请求号的重放之后剩下的唯一冲突，只可能是 (user_id, coupon_id) 部分唯一索引：用户已持有该券。
		// 不按约束名匹配：分区表报告的是分区索引名。占位标记保留（用户确实持有订单）。
		return s.fail(ctx, cmd, events.ReasonRepeat, true, true)
	default:
		return err
	}
	return s.finalizeSuccess(ctx, cmd, orderID)
}

// retryable 判断错误是否应让消息重试而非判终态：下游 transient 故障、ctx 取消/超时，
// 以及任何不是业务错误的未知错误。
func retryable(ctx context.Context, err error) bool {
	if internalapi.IsTransient(err) || ctx.Err() != nil {
		return true
	}
	_, isApp := apperr.As(err)
	return !isApp
}

// checkLimits 复刻 Java 版的持有上限（总数/秒杀数）；超限返回 ReasonLimitExceeded。
func (s *Service) checkLimits(ctx context.Context, userID int64) (events.Reason, error) {
	if s.opts.MaxActiveCoupons <= 0 && s.opts.MaxActiveSeckill <= 0 {
		return "", nil
	}
	c, err := orders.CountsByUser(ctx, s.db, userID)
	if err != nil {
		return "", err
	}
	if (s.opts.MaxActiveCoupons > 0 && c.Total >= s.opts.MaxActiveCoupons) ||
		(s.opts.MaxActiveSeckill > 0 && c.Seckill >= s.opts.MaxActiveSeckill) {
		return events.ReasonLimitExceeded, nil
	}
	return "", nil
}

// persist 在一个事务里写订单与站内通知，返回订单号。同一请求号的重放被 ON CONFLICT 吸收并返回已有订单号
// （此时不会再写第二条通知）。真正的部分唯一索引冲突以错误返回，满足 dbx.IsUniqueViolation(err, "")。
func (s *Service) persist(ctx context.Context, cmd events.SeckillOrderCreate, info internalapi.CouponInfo) (int64, error) {
	now := s.now()
	validDays := max(info.ValidDays, 1)
	var orderID int64
	err := dbx.WithTx(ctx, s.db, dbx.TxOptions{}, func(tx pgx.Tx) error {
		order := orders.NewOrder{
			ID: s.ids.Next(), UserID: cmd.UserID, CouponID: cmd.CouponID, RequestID: cmd.RequestID,
			CouponName: info.Name, CouponType: info.Type, FaceValue: info.FaceValue.String(), MinSpend: info.MinSpend.String(),
			Source: orders.SourceSeckill, Now: now, ExpireAt: now.AddDate(0, 0, validDays),
		}
		id, inserted, err := orders.Insert(ctx, tx, order)
		if err != nil {
			return err
		}
		if !inserted {
			prior, ok, err := orders.FindByRequest(ctx, tx, cmd.UserID, cmd.RequestID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("order for request %s vanished after conflict", cmd.RequestID)
			}
			orderID = prior.ID
			return nil
		}
		orderID = id
		return orders.InsertNotification(ctx, tx, s.ids.Next(), cmd.UserID, "SECKILL_SUCCESS", "秒杀成功",
			fmt.Sprintf("恭喜你抢到了「%s」，请在 %s 前使用。", info.Name, order.ExpireAt.Format("2006-01-02")), id, now)
	})
	if dbx.IsUniqueViolation(err, "") {
		// 两个并发的重放同时插入同一请求号时，PostgreSQL 可能先撞上部分唯一索引而不是 ON CONFLICT 的仲裁索引。
		// 区分「真的已持有别的订单」与「同一请求的重放」：只有后者在库里找得到本请求号的订单。
		prior, found, ferr := orders.FindByRequest(ctx, s.db, cmd.UserID, cmd.RequestID)
		if ferr != nil {
			return 0, ferr
		}
		if found {
			return prior.ID, nil
		}
	}
	if err != nil {
		return 0, err
	}
	return orderID, nil
}

// finalizeSuccess 把请求置为 SUCCESS 并通知钩子。若请求已被 sweeper/入队失败判为 FAILED（订单迟到），
// 订单会被取消、库存被归还——否则库存会多出一份。
func (s *Service) finalizeSuccess(ctx context.Context, cmd events.SeckillOrderCreate, orderID int64) error {
	code, err := s.runAdvance(ctx, cmd.RequestID, cmd.UserID, cmd.CouponID, events.StatusSuccess, orderID)
	if err != nil {
		return err
	}
	switch code {
	case advApplied:
		s.notify(ctx, ResultEvent{RequestID: cmd.RequestID, UserID: cmd.UserID, CouponID: cmd.CouponID, Success: true, OrderID: orderID})
	case advFailedExists:
		logx.From(ctx).Warn("order landed after the request was already failed; reclaiming it", "order_id", orderID)
		return s.lateCleanup(ctx, cmd)
	}
	return nil
}

// reconcileFailed 处理对已 FAILED 请求的重复投递：若竟然有该请求的订单（迟到落库的孤儿），回收它。
// 没有订单是常态，直接提交。
func (s *Service) reconcileFailed(ctx context.Context, cmd events.SeckillOrderCreate) error {
	_, found, err := orders.FindByRequest(ctx, s.db, cmd.UserID, cmd.RequestID)
	if err != nil || !found {
		return err
	}
	logx.From(ctx).Warn("failed request still owns an order; reclaiming it")
	return s.lateCleanup(ctx, cmd)
}

// lateCleanup 取消孤儿订单并（幂等地）归还库存。Redis 侧在置 FAILED 时已经回滚过，这里不再碰。
func (s *Service) lateCleanup(ctx context.Context, cmd events.SeckillOrderCreate) error {
	if _, err := orders.CancelByRequest(ctx, s.db, cmd.UserID, cmd.RequestID, s.now()); err != nil {
		return err
	}
	return s.releaseStock(ctx, cmd.RequestID, cmd.CouponID)
}

// releaseStock 调 coupon 的 stock/release。transient 故障与 ctx 取消返回给调用方重试；
// 其余业务错误（券已删除等）只记日志——重试不会让它变好。
func (s *Service) releaseStock(ctx context.Context, requestID string, couponID int64) error {
	_, err := s.coupon.ReleaseStock(ctx, internalapi.ReleaseRequest{RequestID: requestID, CouponID: couponID})
	switch {
	case err == nil:
		return nil
	case retryable(ctx, err):
		return fmt.Errorf("release stock: %w", err)
	default:
		logx.From(ctx).Error("stock release rejected; not retrying", "request_id", requestID, "coupon_id", couponID, "err", err)
		return nil
	}
}

// fail 把请求置为 FAILED(reason)：可选地先归还 DB 库存（幂等），再经 rollback.lua 回补 Redis 并通知钩子。
// release 为 true 时若归还因 transient 故障失败则返回错误让消息重试（此时 Redis 状态尚未改变）。
func (s *Service) fail(ctx context.Context, cmd events.SeckillOrderCreate, reason events.Reason, release, keepMarker bool) error {
	if release {
		if err := s.releaseStock(ctx, cmd.RequestID, cmd.CouponID); err != nil {
			return err
		}
	}
	return s.rollbackAndNotify(ctx, cmd.RequestID, cmd.UserID, cmd.CouponID, reason, keepMarker)
}

// rollbackAndNotify 执行失败终态的 Redis 回滚，并仅在本次调用完成迁移时通知钩子。
func (s *Service) rollbackAndNotify(ctx context.Context, requestID string, userID, couponID int64, reason events.Reason, keepMarker bool) error {
	code, err := s.runRollback(ctx, requestID, couponID, userID, reason, keepMarker)
	if err != nil {
		return err
	}
	switch code {
	case rbApplied:
		s.notify(ctx, ResultEvent{RequestID: requestID, UserID: userID, CouponID: couponID, Reason: reason})
	case rbSuccessExists:
		logx.From(ctx).Warn("request already succeeded; refusing to fail it", "request_id", requestID, "reason", reason)
	case rbHashMissing:
		logx.From(ctx).Warn("request hash expired before it could be failed", "request_id", requestID, "reason", reason)
	}
	return nil
}
