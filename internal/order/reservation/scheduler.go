package reservation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// finalizeTimeout 是把调用结果写回数据库的超时；它独立于 Run 的 ctx，保证“入口已受理”这个事实
// 即使在关闭过程中也能记录下来。
const finalizeTimeout = 5 * time.Second

// 终态失败的文案。
const (
	repeatMessage  = "已存在该券的秒杀记录，请在「我的券」中查看是否已抢到"
	expiredMessage = "活动已结束仍未触发"
	staleMessage   = "预约处理超时"
	retryExhausted = "（已达重试上限）"
)

// Scheduler 是预约到点调度器：认领到点的 PENDING 预约，代用户调用秒杀入口，把结果写回预约。
// 多个副本同时运行是安全的：认领是单条 FOR UPDATE SKIP LOCKED 语句，每条预约每一轮只会被一个副本认领。
//
// 崩溃安全：认领后、写回前进程退出，预约停在 FIRING；超过 StaleFiringTimeout 后由任一副本回收
// （消耗一次重试额度并退避重试）。重试时若入口其实已经受理过，入口会返回 RepeatSeckill，
// 预约据此终结为 FAILED 并提示用户去「我的券」确认——不会重复扣库存、重复下单。
type Scheduler struct {
	svc    *Service
	sk     Seckiller
	cfg    Config
	reader ResultReader
}

// SchedulerOption 调整 Scheduler。
type SchedulerOption func(*Scheduler)

// WithResultReader 启用对长时间停在 QUEUED 的预约的结果回查，兜底结果事件丢失。
func WithResultReader(r ResultReader) SchedulerOption { return func(s *Scheduler) { s.reader = r } }

// NewScheduler 创建调度器；cfg 不合法时返回错误。
func NewScheduler(svc *Service, sk Seckiller, cfg Config, opts ...SchedulerOption) (*Scheduler, error) {
	if svc == nil || sk == nil {
		return nil, errors.New("reservation: Service 与 Seckiller 都不能为空")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("reservation: 配置不合法: %w", err)
	}
	s := &Scheduler{svc: svc, sk: sk, cfg: cfg}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Run 按 ScanInterval 循环调度，直到 ctx 取消（返回 nil）。单轮失败只记日志，不会退出循环。
// Enabled 为 false 时只阻塞到 ctx 取消，便于作为 app.Worker 统一装配。
func (s *Scheduler) Run(ctx context.Context) error {
	log := logx.From(ctx)
	if !s.cfg.Enabled {
		log.Info("预约调度已禁用", "env", "CK_ORDER_RESERVATION_ENABLED")
		<-ctx.Done()
		return nil
	}
	if s.reader == nil {
		log.Info("预约调度未配置 ResultReader：QUEUED 预约只靠结果事件回写")
	}
	ticker := time.NewTicker(s.cfg.ScanInterval)
	defer ticker.Stop()
	for {
		if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			log.Error("预约调度一轮失败", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// RunOnce 执行一轮调度：回收卡住的 FIRING → 认领并触发到点预约（直到认领不满一批）→ 回查 QUEUED。
// 各步骤互不阻断，错误合并返回。导出供运维脚本与测试单步驱动。
func (s *Scheduler) RunOnce(ctx context.Context) error {
	var errs []error
	if _, err := s.recoverStale(ctx); err != nil {
		errs = append(errs, err)
	}
	if _, err := s.fireDue(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.reconcileQueued(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// fireDue 反复认领到点的预约并触发，直到某次认领不满一批；返回触发的条数。
func (s *Scheduler) fireDue(ctx context.Context) (int, error) {
	total := 0
	for ctx.Err() == nil {
		batch, err := claimDue(ctx, s.svc.db, s.svc.now(), s.cfg.BatchSize)
		if err != nil {
			return total, err
		}
		s.fireBatch(ctx, batch)
		total += len(batch)
		if len(batch) < s.cfg.BatchSize {
			break
		}
	}
	return total, nil
}

// fireBatch 用至多 Concurrency 个 goroutine 处理一批已认领的预约，返回时全部结束。
// ctx 取消后尚未开始处理的预约保持 FIRING，由崩溃回收接手。
func (s *Scheduler) fireBatch(ctx context.Context, batch []claimed) {
	if len(batch) == 0 {
		return
	}
	jobs := make(chan claimed)
	var wg sync.WaitGroup
	for range min(s.cfg.Concurrency, len(batch)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				s.fireOne(ctx, c)
			}
		}()
	}
dispatch:
	for _, c := range batch {
		select {
		case jobs <- c:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
}

// fireOne 处理一条已认领（FIRING）的预约：确认仍归自己 → 调用入口 → 按结果写回。
func (s *Scheduler) fireOne(ctx context.Context, c claimed) {
	log := logx.From(ctx).With("reservation_id", c.id, "user_id", c.userID, "coupon_id", c.couponID)

	ok, err := touch(ctx, s.svc.db, c, s.svc.now())
	if err != nil {
		log.Error("刷新认领失败，保持 FIRING 等待回收", "err", err)
		return
	}
	if !ok {
		log.Warn("预约已被回收或接手，放弃本轮触发")
		return
	}

	rid, enterErr := s.enter(ctx, c)
	if enterErr != nil && ctx.Err() != nil {
		log.Warn("关闭中，入口结果未知，保持 FIRING 等待回收", "err", enterErr)
		return
	}

	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
	defer cancel()
	if err := s.finalize(fctx, c, rid, enterErr); err != nil {
		log.Error("写回触发结果失败，保持 FIRING 等待回收", "err", err, "enter_err", enterErr)
	}
}

// enter 带超时调用秒杀入口；入口 panic 视为错误（调用方已被认领，不能让进程崩在这里）。
func (s *Scheduler) enter(ctx context.Context, c claimed) (rid string, err error) {
	ectx, cancel := context.WithTimeout(ctx, s.cfg.EnterTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			rid, err = "", fmt.Errorf("秒杀入口 panic: %v", r)
		}
	}()
	rid, err = s.sk.Enter(ectx, c.userID, c.couponID)
	if err == nil && rid == "" {
		err = errors.New("秒杀入口受理但没有返回 requestId")
	}
	return rid, err
}

// finalize 按入口结果把预约迁出 FIRING。所有写入都带 fence（认领时的 retry_count）。
func (s *Scheduler) finalize(ctx context.Context, c claimed, rid string, enterErr error) error {
	fence := c.retryCount
	ch := change{id: c.id, now: s.svc.now(), fence: &fence}
	var note func(Reservation) notification.Notification

	switch kind := classify(enterErr); kind {
	case outcomeQueued:
		ch.to, ch.requestID = StatusQueued, &rid
	case outcomeRepeat:
		code, reason := apperr.CodeRepeatSeckill, string(events.ReasonRepeat)
		ch.to, ch.failCode, ch.failReason = StatusFailed, &code, &reason
		note = func(r Reservation) notification.Notification { return failedNote(r, repeatMessage, true) }
	case outcomeExpired:
		code, msg := apperr.CodeActivityEnded, expiredMessage
		ch.to, ch.failCode, ch.failReason = StatusExpired, &code, &msg
		note = expiredNote
	case outcomeTerminal:
		ae := apperr.Resolve(enterErr)
		code, reason := ae.Code, string(events.ReasonForError(enterErr))
		ch.to, ch.failCode, ch.failReason = StatusFailed, &code, &reason
		note = func(r Reservation) notification.Notification { return failedNote(r, ae.Message, true) }
	case outcomeRetry:
		if c.retryCount+1 >= s.cfg.MaxRetry {
			code, text := retryFailure(enterErr)
			reason := string(events.ReasonForError(enterErr))
			ch.to, ch.failCode, ch.failReason = StatusFailed, &code, &reason
			note = func(r Reservation) notification.Notification { return failedNote(r, text, true) }
		} else {
			s.scheduleRetry(ctx, &ch, c.retryCount)
		}
	}

	moved, err := s.svc.transition(ctx, ch, note)
	if err != nil {
		return err
	}
	if !moved {
		logx.From(ctx).Warn("预约已被其他路径迁移，丢弃本轮触发结果",
			"reservation_id", c.id, "to", ch.to, "request_id", rid)
	}
	return nil
}

// scheduleRetry 把 ch 配置成“退回 PENDING、退避后重试、消耗一次重试额度”。
func (s *Scheduler) scheduleRetry(ctx context.Context, ch *change, retryCount int) {
	next := ch.now.Add(s.cfg.retryDelay(retryCount))
	ch.to, ch.triggerAt, ch.bumpRetry = StatusPending, &next, true
	logx.From(ctx).Info("预约触发瞬态失败，退避后重试", "reservation_id", ch.id, "retry", retryCount+1, "next_trigger_at", next)
}

// retryFailure 返回重试耗尽时的失败码与通知文案：保留最后一次业务错误的码与文案，非业务错误归为系统繁忙。
func retryFailure(err error) (int, string) {
	if ae, ok := apperr.As(err); ok {
		return ae.Code, ae.Message + retryExhausted
	}
	return apperr.CodeSysError, "系统繁忙，请稍后再试" + retryExhausted
}

// outcome 是秒杀入口调用结果的分类。
type outcome int

const (
	outcomeQueued   outcome = iota // 受理，等待建单结果
	outcomeRepeat                  // 用户已占位：已有秒杀记录
	outcomeExpired                 // 活动已结束
	outcomeTerminal                // 终态业务失败（库存不足、券停用等）
	outcomeRetry                   // 瞬态失败：退避重试
)

// classify 把入口返回的错误分类。除了明确的“瞬态业务码”之外，4xx 业务错误都是终态；
// 5xx 业务错误与所有非业务错误（网络、超时、panic）都是瞬态。
func classify(err error) outcome {
	switch {
	case err == nil:
		return outcomeQueued
	case errors.Is(err, apperr.RepeatSeckill):
		return outcomeRepeat
	case errors.Is(err, apperr.ActivityEnded):
		return outcomeExpired
	}
	ae, ok := apperr.As(err)
	if !ok || ae.HTTPStatus >= http.StatusInternalServerError {
		return outcomeRetry
	}
	switch ae.Code {
	case apperr.CodeCoolingDown, apperr.CodeNotPreheated, apperr.CodeNotStarted, apperr.CodeRateLimited:
		return outcomeRetry
	}
	return outcomeTerminal
}

// recoverStale 回收认领后停留超过 StaleFiringTimeout 的 FIRING 预约（认领方已崩溃）：
// 还有重试额度则退回 PENDING 退避重试，否则置 FAILED 并通知用户。返回回收的条数。
func (s *Scheduler) recoverStale(ctx context.Context) (int, error) {
	now := s.svc.now()
	recovered := 0
	err := dbx.WithTx(ctx, s.svc.db, dbx.TxOptions{}, func(tx pgx.Tx) error {
		recovered = 0
		stale, err := lockStale(ctx, tx, now.Add(-s.cfg.StaleFiringTimeout), s.cfg.BatchSize)
		if err != nil {
			return err
		}
		for _, c := range stale {
			ch := change{id: c.id, now: now}
			var note func(Reservation) notification.Notification
			if c.retryCount+1 >= s.cfg.MaxRetry {
				code, reason := apperr.CodeSysError, string(events.ReasonTimeout)
				ch.to, ch.failCode, ch.failReason = StatusFailed, &code, &reason
				note = func(r Reservation) notification.Notification {
					return failedNote(r, staleMessage+retryExhausted, true)
				}
			} else {
				s.scheduleRetry(ctx, &ch, c.retryCount)
			}
			if _, err := s.svc.moveAndNotify(ctx, tx, ch, note); err != nil {
				return err
			}
			recovered++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("回收卡住的预约: %w", err)
	}
	if recovered > 0 {
		logx.From(ctx).Warn("回收卡住的 FIRING 预约", "count", recovered)
	}
	return recovered, nil
}

// reconcileQueued 回查长时间停在 QUEUED 的预约的结果（需要 ResultReader），
// 查到终态就按结果事件同样的路径回写。回查是兜底：正常情况下结果事件早已把它们迁走。
func (s *Scheduler) reconcileQueued(ctx context.Context) error {
	if s.reader == nil {
		return nil
	}
	now := s.svc.now()
	refs, err := lockQueuedStale(ctx, s.svc.db, now.Add(-s.cfg.QueuedSyncAfter), now, s.cfg.BatchSize)
	if err != nil {
		return err
	}
	var errs []error
	for _, ref := range refs {
		if ctx.Err() != nil {
			break
		}
		ev, found, err := s.reader.Result(ctx, ref.requestID)
		if err != nil {
			logx.From(ctx).Warn("回查秒杀结果失败", "reservation_id", ref.id, "request_id", ref.requestID, "err", err)
			continue
		}
		if !found {
			continue
		}
		ev.RequestID, ev.UserID, ev.CouponID = ref.requestID, ref.userID, ref.couponID
		if err := s.svc.OnSeckillResult(ctx, ev); err != nil {
			errs = append(errs, fmt.Errorf("回写预约 %d 的回查结果: %w", ref.id, err))
		}
	}
	return errors.Join(errs...)
}
