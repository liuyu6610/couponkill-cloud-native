// Package reservation 实现“预约帮抢”：用户在秒杀开售前预约，到点由调度器代用户走与手动秒杀
// 同一条入口流水线（Seckiller），结果回写预约并产生站内通知。
//
// 数据在 ord.reservations（状态机见 status.go 与 README）。核心不变式：
//  1. 同一用户同一张券同时至多一个进行中（PENDING/FIRING/QUEUED）预约：由部分唯一索引保证。
//  2. 每条预约每一轮至多调用一次 Seckiller.Enter：认领是单条 SKIP LOCKED 语句（多副本并发安全），
//     之后的所有写入都带 fence（retry_count），被崩溃回收后迟到的写入会落空。
//  3. 状态只能沿 transitions 迁移，终态不可再变；通知与状态迁移在同一事务，迁移成功才写通知，
//     因此任何重放（结果事件重复投递、调度重复回收）都不会产生第二条通知。
package reservation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Service 是预约的业务入口：创建/取消/查询，以及结果回写。并发安全。
type Service struct {
	db      DB
	ids     IDGenerator
	coupons CouponLookup
	now     func() time.Time
}

// Option 调整 Service。
type Option func(*Service)

// WithClock 注入时钟（测试用）。
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// NewService 创建 Service。
func NewService(db DB, ids IDGenerator, coupons CouponLookup, opts ...Option) *Service {
	s := &Service{db: db, ids: ids, coupons: coupons, now: func() time.Time { return time.Now().UTC() }}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Create 为 userID 创建对 couponID 的预约，规则沿用 Java：
//   - 券必须存在（404）、是秒杀券且启用中（停用 409）、已配置活动时间窗；
//   - 活动已结束 409（ActivityEnded）；已开售（now >= 开售时间）400，应直接秒杀；
//   - 同一用户对同一券已有进行中的预约 409（ErrDuplicate），由部分唯一索引兜底，并发创建也只会成功一个。
//
// 触发时间 trigger_at 取券的开售时间。
func (s *Service) Create(ctx context.Context, userID, couponID int64) (Reservation, error) {
	if userID <= 0 || couponID <= 0 {
		return Reservation{}, apperr.InvalidRequest.WithMessage("userId/couponId 不能为空")
	}
	coupon, err := s.coupons.Coupon(ctx, couponID)
	if err != nil {
		if _, ok := apperr.As(err); ok {
			return Reservation{}, err
		}
		return Reservation{}, apperr.CouponServiceUnavailable.WithCause(err)
	}
	now := s.now()
	if err := checkReservable(coupon, now); err != nil {
		return Reservation{}, err
	}

	r := Reservation{
		ID: s.ids.Next(), UserID: userID, CouponID: couponID,
		Status: StatusPending, ReserveAt: now, TriggerAt: coupon.SeckillStartAt,
	}
	if err := insertPending(ctx, s.db, r); err != nil {
		if dbx.IsUniqueViolation(err, "uk_reservations_active_user_coupon") {
			return Reservation{}, ErrDuplicate.WithCause(err)
		}
		return Reservation{}, err
	}
	logx.From(ctx).Info("创建预约帮抢", "reservation_id", r.ID, "coupon_id", couponID, "trigger_at", r.TriggerAt)
	return getByID(ctx, s.db, r.ID, userID)
}

// checkReservable 校验券是否允许预约（Java 的创建规则）。
func checkReservable(c Coupon, now time.Time) error {
	if c.Type != CouponTypeSeckill {
		return apperr.InvalidRequest.WithMessage("仅秒抢券支持预约帮抢")
	}
	if c.Status != CouponStatusEnabled {
		return apperr.CouponDisabled.WithMessage("优惠券已失效")
	}
	if c.SeckillStartAt == nil || c.SeckillEndAt == nil {
		return apperr.InvalidRequest.WithMessage("该秒杀券尚未配置活动时间窗，暂不可预约")
	}
	if !now.Before(*c.SeckillEndAt) {
		return apperr.ActivityEnded
	}
	if !now.Before(*c.SeckillStartAt) {
		return apperr.InvalidRequest.WithMessage("活动已开售，请直接秒杀，不可再预约")
	}
	return nil
}

// cancelAttempts 是取消时与“FIRING→PENDING 回退”竞争的最大尝试次数。
const cancelAttempts = 3

// Cancel 取消 userID 的预约：只有属主、且仍是 PENDING 时可取消。
// 不存在或不属于该用户返回 ErrNotFound；已被认领/终态返回 409（含当前状态）。
func (s *Service) Cancel(ctx context.Context, userID, id int64) error {
	for range cancelAttempts {
		_, moved, err := move(ctx, s.db, change{id: id, to: StatusCancelled, now: s.now(), userID: userID})
		if err != nil {
			return err
		}
		if moved {
			logx.From(ctx).Info("取消预约帮抢", "reservation_id", id)
			return nil
		}
		cur, err := getByID(ctx, s.db, id, userID)
		if err != nil {
			return err
		}
		if cur.Status != StatusPending {
			return errNotCancellable(cur.Status)
		}
		// 刚好从 FIRING 退回了 PENDING（瞬态失败重试）：再试一次。
	}
	return apperr.New(apperr.CodeInvalidRequest, http.StatusConflict, "预约状态正在变化，请稍后重试")
}

// Get 返回 userID 的一条预约；不存在或不属于该用户返回 ErrNotFound。
func (s *Service) Get(ctx context.Context, userID, id int64) (Reservation, error) {
	return getByID(ctx, s.db, id, userID)
}

// Mine 返回 userID 的预约，最新在前；page 从 1 开始。
func (s *Service) Mine(ctx context.Context, userID int64, page, size int) ([]Reservation, int64, error) {
	return listByUser(ctx, s.db, userID, page, size)
}

// OnSeckillResult 把秒杀建单结果回写到对应的预约：按 request_id 找到预约，
// 成功 → SUCCESS（记录 order_id），失败 → FAILED（记录失败码与原因），并在同一事务里写通知。
//
// 幂等：重复投递时预约已是终态，不再迁移也不再写通知；找不到对应预约（用户是手动秒杀）直接返回 nil。
// 结果事件可能先于调度器的 QUEUED 回写到达：此时按 (user, coupon) 兜底匹配仍在 FIRING 的预约。
// 返回错误表示事务失败，调用方应让消息重试。
func (s *Service) OnSeckillResult(ctx context.Context, ev ResultEvent) error {
	if ev.RequestID == "" || ev.UserID <= 0 || ev.CouponID <= 0 {
		return apperr.InvalidRequest.WithMessage("秒杀结果缺少 requestId/userId/couponId")
	}
	now := s.now()
	return dbx.WithTx(ctx, s.db, dbx.TxOptions{}, func(tx pgx.Tx) error {
		cur, found, err := lockForResult(ctx, tx, ev)
		if err != nil {
			return err
		}
		if !found {
			logx.From(ctx).Debug("秒杀结果没有对应的预约（手动秒杀）", "request_id", ev.RequestID)
			return nil
		}
		if cur.Status != StatusFiring && cur.Status != StatusQueued {
			logx.From(ctx).Debug("预约已是终态，忽略重复的秒杀结果", "reservation_id", cur.ID, "status", cur.Status)
			return nil
		}
		ch := change{id: cur.ID, now: now, requestID: &ev.RequestID}
		var note func(Reservation) notification.Notification
		if ev.Success {
			ch.to = StatusSuccess
			if ev.OrderID > 0 {
				ch.orderID = &ev.OrderID
			}
			orderID := ev.OrderID
			note = func(r Reservation) notification.Notification { return successNote(r, orderID) }
		} else {
			f := failureOf(ev.Reason)
			code, reason := f.code, string(f.reason)
			ch.to, ch.failCode, ch.failReason = StatusFailed, &code, &reason
			note = func(r Reservation) notification.Notification { return failedNote(r, f.text, f.known) }
		}
		_, err = s.moveAndNotify(ctx, tx, ch, note)
		return err
	})
}

// failure 描述一次终态失败：失败码、写入 fail_reason 的原因码（events.Reason 词表，前端据此映射中文）、
// 通知里给用户看的文案；known 表示原因是明确的业务原因（否则通知用通用文案）。
type failure struct {
	code   int
	reason events.Reason
	text   string
	known  bool
}

// failureOf 把结果事件里的失败原因翻译为 failure；空或未知的原因归为 SYSTEM_ERROR。
func failureOf(reason string) failure {
	r := events.Reason(reason)
	if r.Valid() && r != events.ReasonSystemError {
		e := r.AppError()
		return failure{code: e.Code, reason: r, text: e.Message, known: true}
	}
	return failure{code: apperr.CodeSysError, reason: events.ReasonSystemError, text: "秒杀履约失败"}
}

// moveAndNotify 在 tx 内迁移状态，迁移真正发生时才写通知（note 为 nil 则不写）。
func (s *Service) moveAndNotify(ctx context.Context, tx pgx.Tx, ch change, note func(Reservation) notification.Notification) (bool, error) {
	r, moved, err := move(ctx, tx, ch)
	if err != nil || !moved {
		return false, err
	}
	if note != nil {
		n := note(r)
		n.ID, n.UserID, n.CreatedAt = s.ids.Next(), r.UserID, ch.now
		refID := r.ID
		n.RefID = &refID
		if err := notification.Insert(ctx, tx, n); err != nil {
			return false, err
		}
	}
	logx.From(ctx).Info("预约状态迁移", "reservation_id", r.ID, "to", ch.to, "retry_count", r.RetryCount)
	return true, nil
}

// transition 在独立事务里完成一次迁移与通知，返回是否真正迁移。
func (s *Service) transition(ctx context.Context, ch change, note func(Reservation) notification.Notification) (bool, error) {
	var moved bool
	err := dbx.WithTx(ctx, s.db, dbx.TxOptions{}, func(tx pgx.Tx) error {
		var err error
		moved, err = s.moveAndNotify(ctx, tx, ch, note)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("预约 %d 迁移到 %s: %w", ch.id, ch.to, err)
	}
	return moved, nil
}

// 通知文案沿用 Java。

func successNote(r Reservation, orderID int64) notification.Notification {
	content := fmt.Sprintf("券 %d 已抢到", r.CouponID)
	if orderID > 0 {
		content += fmt.Sprintf("，订单 %d", orderID)
	}
	return notification.Notification{Type: notification.TypeReservationSuccess, Title: "预约帮抢成功", Content: content}
}

// failedNote 构造失败通知：known 为 true 时带上具体原因，否则用 Java 的通用文案。
func failedNote(r Reservation, msg string, known bool) notification.Notification {
	content := fmt.Sprintf("券 %d 履约失败，请查看「我的预约」", r.CouponID)
	if known {
		content = fmt.Sprintf("券 %d：%s", r.CouponID, msg)
	}
	return notification.Notification{Type: notification.TypeReservationFailed, Title: "预约帮抢失败", Content: content}
}

func expiredNote(r Reservation) notification.Notification {
	return notification.Notification{
		Type: notification.TypeReservationExpired, Title: "预约已过期",
		Content: fmt.Sprintf("券 %d 活动已结束，预约失效", r.CouponID),
	}
}
