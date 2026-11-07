package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// 同步默认值。
const (
	defaultLockTTL     = 60 * time.Second
	minLockTTL         = 10 * time.Second
	defaultPushTimeout = 5 * time.Second
	recordTimeout      = 5 * time.Second
	maxErrorLen        = 500
	syncSource         = "connector"
	skipMessage        = "平台未返回可用精确库存，且无法推导目标值"
)

// SyncRepo 是同步用到的仓储能力。
type SyncRepo interface {
	GetBinding(ctx context.Context, id int64) (domain.Binding, error)
	ListSyncCandidates(ctx context.Context) ([]domain.Binding, error)
	RecordSync(ctx context.Context, id int64, o domain.SyncOutcome) (domain.Binding, error)
}

// Adapters 是按健康状态把关的适配器集合（*platforms.Registry 实现它）。
type Adapters interface {
	Get(ctx context.Context, p domain.Platform) (platforms.PlatformAdapter, error)
	Health(ctx context.Context) []domain.Health
}

// SyncConfig 配置 SyncService。
type SyncConfig struct {
	// Budget 是定时批次的时间预算：仅在每条绑定开始前检查，在途的一条会跑完；<=0 表示不限。
	Budget time.Duration
	// LockTTL 是同步锁的租约时长，默认 60s，下限 10s。每条绑定开始前与写库存前都会续满。
	LockTTL time.Duration
	// PushTimeout 是向 coupon 推送库存的超时，默认 5s（必须远小于 LockTTL）。
	PushTimeout time.Duration
	// Now 是时钟（测试注入）；既用于预算计时，也是写入 last_sync_at 的时间。
	Now func() time.Time
}

// SyncService 把平台库存同步给 coupon 服务。
//
// 不变式（测试逐条覆盖）：
//  1. 任一时刻只有一个持锁者在推送：定时批次、手工同步、其它实例共用 ck:lock:connector-sync。
//  2. 每条绑定开始前、写 coupon 库存前都先续租并确认锁仍归自己；确认失败（含 Redis 不可用）
//     就停止本轮，已读到的平台库存也不写，剩余绑定原样保留（last_sync_at 不变，下一轮排在最前）。
//  3. 处理顺序是“最久未同步优先”；成功、失败、跳过都刷新 last_sync_at，
//     所以一条慢而持续失败的绑定不会每轮都排第一把预算吃光、饿死其它绑定。
//  4. 推送 coupon 的 SyncStock 客户端不自动重试：失败只记 FAIL，由下一轮重新推送，
//     last_stock 只在确认成功后才更新。自动与普通手工同步用「只降不升」(min) 模式：平台读数可能是恒定值
//     （mock 恒为 500）或过期值，按它设置会把已卖出的库存重置回去造成超卖；只有管理员显式 force 才用 set 抬高。
//  5. 调用方 ctx 取消（服务关闭）不是平台故障：不记 FAIL，直接结束，锁照常释放。
type SyncService struct {
	repo        SyncRepo
	adapters    Adapters
	coupon      internalapi.StockSyncer
	locker      Locker
	budget      time.Duration
	stepTimeout time.Duration
	pushTimeout time.Duration
	now         func() time.Time
}

// NewSyncService 创建同步服务。
func NewSyncService(repo SyncRepo, adapters Adapters, coupon internalapi.StockSyncer, locker Locker, cfg SyncConfig) *SyncService {
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = defaultLockTTL
	}
	cfg.LockTTL = max(cfg.LockTTL, minLockTTL)
	if cfg.PushTimeout <= 0 {
		cfg.PushTimeout = defaultPushTimeout
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &SyncService{
		repo: repo, adapters: adapters, coupon: coupon, locker: locker,
		budget: cfg.Budget,
		// 单条绑定里平台调用的上限取 TTL 的一半：即使平台很慢，写库存前的续租也一定发生在租约过期之前。
		stepTimeout: cfg.LockTTL / 2,
		pushTimeout: min(cfg.PushTimeout, cfg.LockTTL/4),
		now:         cfg.Now,
	}
}

// RunScheduled 执行一轮定时同步（受 Budget 限制）。拿不到锁（别人在同步）返回 ran=false 且无错误，
// 定时任务据此静默跳过；加锁本身失败（如 Redis 不可用）返回错误且不做任何事。
func (s *SyncService) RunScheduled(ctx context.Context) (res domain.SyncBatchResult, ran bool, err error) {
	lease, err := s.locker.TryAcquire(ctx)
	switch {
	case domain.IsSyncBusy(err):
		logx.From(ctx).Debug("scheduled stock sync skipped: lock held elsewhere")
		return res, false, nil
	case err != nil:
		return res, false, err
	}
	defer lease.Release(ctx)
	res, err = s.batch(ctx, lease, false, s.budget)
	return res, true, err
}

// SyncAll 手工同步全部启用的绑定：同样持有同步锁，不设时间预算，但逐条续租。
// 锁被占用返回 ErrSyncBusy（HTTP 409）；中途丢锁时剩余绑定计入 skipped。
// force=true 时以 set 模式推送（可以抬高 coupon 库存），否则 min 模式（只降不升），见不变式 4。
func (s *SyncService) SyncAll(ctx context.Context, force bool) (domain.SyncBatchResult, error) {
	lease, err := s.locker.TryAcquire(ctx)
	if err != nil {
		return domain.SyncBatchResult{}, err
	}
	defer lease.Release(ctx)
	logx.From(ctx).Info("manual stock sync of all bindings", "force", force)
	return s.batch(ctx, lease, force, 0)
}

// SyncOne 手工同步单个绑定。绑定不存在返回 404（先于锁检查）；锁被占用返回 409；
// 写库存前丢锁返回 409（本次未写库存）；平台或推送失败时返回记录了 FAIL 状态的绑定，而不是错误。
func (s *SyncService) SyncOne(ctx context.Context, id int64, force bool) (domain.Binding, error) {
	b, err := s.repo.GetBinding(ctx, id)
	if err != nil {
		return domain.Binding{}, err
	}
	lease, err := s.locker.TryAcquire(ctx)
	if err != nil {
		return domain.Binding{}, err
	}
	defer lease.Release(ctx)
	logx.From(ctx).Info("manual stock sync of one binding", "binding_id", id, "force", force)

	out, kind, err := s.syncBinding(ctx, lease, b, force)
	switch kind {
	case stepLost:
		return domain.Binding{}, domain.ErrSyncLockLost
	case stepAborted:
		return domain.Binding{}, fmt.Errorf("manual sync aborted: %w", ctx.Err())
	}
	return out, err
}

// batch 在持锁状态下按“最久未同步优先”处理启用的绑定。
func (s *SyncService) batch(ctx context.Context, lease Lease, force bool, budget time.Duration) (domain.SyncBatchResult, error) {
	log := logx.From(ctx)
	candidates, err := s.repo.ListSyncCandidates(ctx)
	if err != nil {
		return domain.SyncBatchResult{}, fmt.Errorf("list sync candidates: %w", err)
	}
	res := domain.SyncBatchResult{Force: force}
	start := s.now()
	stopped, deferred := "", 0
	for _, b := range candidates {
		if stopped == "" {
			stopped = s.stopReason(ctx, lease, start, budget)
		}
		if stopped != "" {
			deferred++
			continue
		}
		_, kind, err := s.syncBinding(ctx, lease, b, force)
		if err != nil {
			log.Error("record sync result failed", "binding_id", int64(b.ID), "err", err)
		}
		switch kind {
		case stepOK:
			res.SyncedOK++
		case stepSkip:
			res.Skipped++
		case stepFail:
			res.Failed++
		case stepLost:
			stopped = "lease"
			deferred++
		case stepAborted:
			stopped = "shutdown"
			deferred++
		}
	}
	res.Skipped += deferred
	switch {
	case stopped == "lease":
		log.Warn("stock sync lock lost; stopping this round", "deferred", deferred, "total", len(candidates))
	case stopped == "budget":
		log.Warn("stock sync budget exhausted; remaining bindings deferred to next round", "budget", budget.String(), "deferred", deferred, "total", len(candidates))
	case stopped == "shutdown":
		log.Info("stock sync interrupted by shutdown", "deferred", deferred, "total", len(candidates))
	case res.SyncedOK+res.Failed+res.Skipped > 0:
		log.Info("stock sync round finished", "ok", res.SyncedOK, "failed", res.Failed, "skipped", res.Skipped)
	}
	return res, nil
}

// stopReason 在每条绑定开始前检查是否该停：关闭、预算耗尽、租约丢失。返回空串表示继续。
func (s *SyncService) stopReason(ctx context.Context, lease Lease, start time.Time, budget time.Duration) string {
	switch {
	case ctx.Err() != nil:
		return "shutdown"
	case budget > 0 && s.now().Sub(start) >= budget:
		return "budget"
	}
	if err := lease.Guard(ctx); err != nil {
		logx.From(ctx).Warn("cannot confirm sync lock before next binding", "err", err)
		return "lease"
	}
	return ""
}

// stepKind 是处理一条绑定的结局。
type stepKind int

const (
	stepOK      stepKind = iota // 已推送并记录 SUCCESS
	stepSkip                    // 平台没给精确库存，记录 SKIP
	stepFail                    // 平台或推送失败，记录 FAIL
	stepLost                    // 写库存前确认锁已丢失：什么都没写、没记录
	stepAborted                 // 调用方 ctx 取消：什么都没记录
)

// syncBinding 处理一条绑定：读平台库存 → 解析目标值 → 写前续租 → 推送 coupon → 记录结果。
// 返回的 error 只表示“结果没能写回数据库”；业务上的失败体现在 kind 与记录的 FAIL 状态里。
func (s *SyncService) syncBinding(ctx context.Context, lease Lease, b domain.Binding, force bool) (domain.Binding, stepKind, error) {
	log := logx.From(ctx).With("binding_id", int64(b.ID), "platform", string(b.Platform), "sku", b.ExternalSkuID, "coupon_id", int64(b.CouponID))

	stepCtx, cancel := context.WithTimeout(ctx, s.stepTimeout)
	snap, err := s.readStock(stepCtx, b)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return b, stepAborted, nil
		}
		log.Warn("platform stock read failed", "err", err)
		return s.record(ctx, b, stepFail, domain.SyncOutcome{Status: domain.SyncFail, Error: errText(err)})
	}

	target, ok := resolveTarget(snap)
	if !ok {
		log.Info("platform returned no usable exact stock; skipping to avoid inflating")
		msg := skipMessage
		return s.record(ctx, b, stepSkip, domain.SyncOutcome{Status: domain.SyncSkip, Error: &msg})
	}

	if err := lease.Guard(ctx); err != nil {
		if ctx.Err() != nil {
			return b, stepAborted, nil
		}
		log.Warn("sync lock lost before writing stock; nothing written", "err", err)
		return b, stepLost, nil
	}

	pushCtx, cancelPush := context.WithTimeout(ctx, s.pushTimeout)
	mode := internalapi.SyncModeMin
	if force {
		mode = internalapi.SyncModeSet
	}
	resp, err := s.coupon.SyncStock(pushCtx, internalapi.SyncRequest{CouponID: int64(b.CouponID), Available: target, Source: syncSource, Mode: mode})
	cancelPush()
	if err != nil {
		if ctx.Err() != nil {
			return b, stepAborted, nil
		}
		log.Warn("push stock to coupon failed; will retry next round", "target", target, "err", err)
		return s.record(ctx, b, stepFail, domain.SyncOutcome{Status: domain.SyncFail, Error: errText(err)})
	}
	applied := resp.Available
	log.Info("stock synced", "mode", mode, "target", target, "applied", applied)
	return s.record(ctx, b, stepOK, domain.SyncOutcome{Status: domain.SyncSuccess, LastStock: &applied})
}

// readStock 通过注册表取适配器（按健康状态把关）并读取平台库存。
func (s *SyncService) readStock(ctx context.Context, b domain.Binding) (domain.StockSnapshot, error) {
	adapter, err := s.adapters.Get(ctx, b.Platform)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	return adapter.Stock(ctx, b.ExternalSkuID)
}

// record 把结果写回绑定。写回用不受取消影响的短超时上下文，避免关闭时丢掉已完成的结果。
func (s *SyncService) record(ctx context.Context, b domain.Binding, kind stepKind, o domain.SyncOutcome) (domain.Binding, stepKind, error) {
	o.At = s.now().UTC()
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	out, err := s.repo.RecordSync(rctx, int64(b.ID), o)
	if err != nil {
		return b, kind, fmt.Errorf("record sync outcome for binding %d: %w", b.ID, err)
	}
	return out, kind, nil
}

// resolveTarget 把平台快照解析成要推送的绝对库存：
// 有精确数量以平台为准（含 0）；没有数量但明确无货则为 0；有货却不精确则无法推导（返回 false，
// 调用方记 SKIP——绝不用默认值虚增库存）。
func resolveTarget(s domain.StockSnapshot) (int64, bool) {
	if s.StockQty != nil && *s.StockQty >= 0 {
		return *s.StockQty, true
	}
	if s.StockAvailable != nil && !*s.StockAvailable {
		return 0, true
	}
	return 0, false
}

// errText 把错误转成写入 last_error 的文本（限长，避免过长的响应片段撑爆列）。
func errText(err error) *string {
	msg := err.Error()
	if errors.Is(err, platforms.ErrDisabled) {
		msg = "平台未启用（缺少凭证）: " + msg
	}
	if r := []rune(msg); len(r) > maxErrorLen {
		msg = string(r[:maxErrorLen])
	}
	return &msg
}
