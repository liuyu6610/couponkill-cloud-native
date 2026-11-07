package seckill

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// sweeperLockName 是清扫器的分布式锁名（键 ck:lock:seckill-sweeper）。
const sweeperLockName = "seckill-sweeper"

// SweeperOptions 配置清扫器；零值字段取默认值。
type SweeperOptions struct {
	// Interval 是两轮之间的间隔，默认 30s（CK_ORDER_SWEEP_INTERVAL）。
	Interval time.Duration
	// Stale 是请求在途多久后算超时，默认 120s（CK_ORDER_SWEEP_STALE）。
	Stale time.Duration
	// Batch 是每轮最多处理的请求数，默认 100（CK_ORDER_SWEEP_BATCH）。
	Batch int
	// LockTTL 是分布式锁的租期（持锁期间自动续租），默认 60s。
	LockTTL time.Duration
}

func (o SweeperOptions) withDefaults() SweeperOptions {
	if o.Interval <= 0 {
		o.Interval = 30 * time.Second
	}
	if o.Stale <= 0 {
		o.Stale = 120 * time.Second
	}
	if o.Batch <= 0 {
		o.Batch = 100
	}
	if o.LockTTL <= 0 {
		o.LockTTL = 60 * time.Second
	}
	return o
}

// Sweeper 定时收尾「卡在在途集合里」的秒杀请求：消费者崩溃、重试耗尽进了死信、入队回滚失败……
// 任何原因导致请求长时间没有到终态，都由它按落库事实裁决：
//
//   - 订单已存在 ⇒ 置 SUCCESS；
//   - 订单不存在 ⇒ 归还 DB 库存（幂等，并在 coupon 侧留下墓碑，迟到的 deduct 会被拒绝）+ rollback.lua，置 FAILED(TIMEOUT)。
//
// 多实例靠 ck:lock:seckill-sweeper 互斥；所有步骤都幂等，失败的条目留在集合里下一轮重试。
type Sweeper struct {
	svc    *Service
	opts   SweeperOptions
	locker *redisx.Locker
}

// NewSweeper 创建清扫器。
func (s *Service) NewSweeper(opts SweeperOptions) *Sweeper {
	return &Sweeper{svc: s, opts: opts.withDefaults(), locker: redisx.NewLocker(s.rdb)}
}

// Run 循环执行清扫直到 ctx 取消（返回 nil）。单轮失败只记日志，不会让 worker 退出。
func (sw *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(sw.opts.Interval)
	defer ticker.Stop()
	for {
		sw.tick(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// tick 执行一轮并隔离错误与 panic。
func (sw *Sweeper) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logx.From(ctx).Error("seckill sweeper panicked", "panic", fmt.Sprint(r))
		}
	}()
	n, err := sw.Once(ctx)
	switch {
	case err != nil && ctx.Err() == nil:
		logx.From(ctx).Error("seckill sweeper round failed", "handled", n, "err", err)
	case n > 0:
		logx.From(ctx).Warn("seckill sweeper finalized stale requests", "count", n)
	}
}

// Once 执行一轮清扫，返回本轮收尾的请求数。没抢到锁（别的实例在跑）返回 0, nil。
func (sw *Sweeper) Once(ctx context.Context) (int, error) {
	handled := 0
	err := sw.locker.Run(ctx, sweeperLockName, sw.opts.LockTTL, func(ctx context.Context) error {
		var err error
		handled, err = sw.round(ctx)
		return err
	})
	if errors.Is(err, redisx.ErrNotAcquired) {
		return 0, nil
	}
	return handled, err
}

// round 取出超时的在途请求（最旧的优先、条数有界）逐个收尾，返回成功收尾的条数与合并后的错误。
func (sw *Sweeper) round(ctx context.Context) (int, error) {
	cutoff := sw.svc.now().Add(-sw.opts.Stale).UnixMilli()
	stale, err := sw.svc.rdb.ZRangeByScore(ctx, keys.SeckillInflight(), &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(cutoff, 10), Offset: 0, Count: int64(sw.opts.Batch),
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("list stale requests: %w", err)
	}
	var errs []error
	handled := 0
	for _, rid := range stale {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		done, err := sw.finalize(logx.With(ctx, "request_id", rid), rid)
		if err != nil {
			errs = append(errs, fmt.Errorf("request %s: %w", rid, err))
			continue
		}
		if done {
			handled++
		}
	}
	return handled, errors.Join(errs...)
}

// finalize 收尾单个请求；done=false 且 err=nil 表示无需处理（已终态、消费者仍在活跃处理等）。
func (sw *Sweeper) finalize(ctx context.Context, requestID string) (done bool, err error) {
	s := sw.svc
	vals, err := s.rdb.HMGet(ctx, keys.SeckillRequest(requestID),
		keys.ReqFieldStatus, keys.ReqFieldUserID, keys.ReqFieldCouponID, keys.ReqFieldUpdatedAt).Result()
	if err != nil {
		return false, fmt.Errorf("read request hash: %w", err)
	}
	status, _ := vals[0].(string)
	if status == "" {
		logx.From(ctx).Warn("in-flight request has no hash (expired); dropping it from the in-flight set")
		return false, s.dropInflight(ctx, requestID)
	}
	if events.Status(status).Terminal() {
		return false, s.dropInflight(ctx, requestID)
	}
	if events.Status(status) == events.StatusProcessing && sw.activelyProcessed(vals[3]) {
		return false, nil
	}
	userID, uerr := strconv.ParseInt(asString(vals[1]), 10, 64)
	couponID, cerr := strconv.ParseInt(asString(vals[2]), 10, 64)
	if uerr != nil || cerr != nil {
		logx.From(ctx).Error("in-flight request hash is corrupt; dropping it", "user_id", vals[1], "coupon_id", vals[2])
		return false, s.dropInflight(ctx, requestID)
	}
	ctx = logx.With(ctx, "user_id", userID, "coupon_id", couponID)

	cmd := events.SeckillOrderCreate{RequestID: requestID, UserID: userID, CouponID: couponID}
	order, found, err := orders.FindByRequest(ctx, s.db, userID, requestID)
	if err != nil {
		return false, err
	}
	if found {
		return true, s.finalizeSuccess(ctx, cmd, order.ID)
	}
	// 没有订单：先归还 DB 库存（并留墓碑，挡住迟到的 deduct），再回滚 Redis。两步都幂等，失败留给下一轮。
	if err := s.releaseStock(ctx, requestID, couponID); err != nil {
		return false, err
	}
	return true, s.rollbackAndNotify(ctx, requestID, userID, couponID, events.ReasonTimeout, false)
}

// activelyProcessed 判断 PROCESSING 状态是否仍然「新鲜」：消费者领取（或重试刷新）后不足 Stale，
// 说明很可能还在处理中，不应抢先收尾（否则会与正在落库的消费者赛跑）。
func (sw *Sweeper) activelyProcessed(updatedAt any) bool {
	ms, err := strconv.ParseInt(asString(updatedAt), 10, 64)
	if err != nil {
		return false
	}
	return sw.svc.now().UnixMilli()-ms < sw.opts.Stale.Milliseconds()
}

func (s *Service) dropInflight(ctx context.Context, requestID string) error {
	if err := s.rdb.ZRem(ctx, keys.SeckillInflight(), requestID).Err(); err != nil {
		return fmt.Errorf("drop from in-flight set: %w", err)
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
