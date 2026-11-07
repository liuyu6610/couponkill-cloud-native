package seckill

import (
	"context"
	_ "embed" // go:embed 内嵌 Lua 脚本
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

//go:embed lua/enter.lua
var enterLua string

//go:embed lua/rollback.lua
var rollbackLua string

//go:embed lua/advance.lua
var advanceLua string

// enter.lua 的返回码（设计 §8）。
const (
	enterOK           = 1
	enterSoldOut      = 0
	enterCoolingDown  = -2
	enterRepeat       = -3
	enterNotPreheated = -4
)

// advance.lua / rollback.lua 的返回码。
const (
	advApplied       = 1
	advAlreadySucc   = 0  // 目标 SUCCESS 时请求已是 SUCCESS
	advSuccessExists = -1 // 目标 PROCESSING 时请求已是 SUCCESS
	advFailedExists  = -2 // 请求已是 FAILED

	rbApplied       = 1
	rbAlreadyDone   = 0
	rbSuccessExists = -1
	rbHashMissing   = -2
)

// CouponAPI 是 seckill 依赖的 coupon 内部 API 子集（接口在使用方定义，*internalapi.CouponClient 满足它）。
type CouponAPI interface {
	internalapi.CouponGetter
	internalapi.StockDeductor
	internalapi.StockReleaser
}

// IDSource 生成订单与通知的 snowflake ID（*idgen.Generator 满足它）。
type IDSource interface {
	Next() int64
}

// DB 是 seckill 需要的数据库能力：事务与查询（*pgxpool.Pool 满足它）。
type DB interface {
	dbx.TxBeginner
	orders.Querier
}

// Deps 是 Service 的外部依赖。
type Deps struct {
	// DB 是 ord schema 所在的连接池。
	DB DB
	// Redis 是 Redis 客户端。
	Redis redis.Cmdable
	// Producer 发布建单命令。
	Producer kafkax.Producer
	// Coupon 是 coupon 内部 API 客户端。
	Coupon CouponAPI
	// IDs 生成订单与通知 ID。
	IDs IDSource
}

// Options 是 Service 的可调参数；零值字段取默认值。
type Options struct {
	// Cooldown 是同一用户对同一张券两次秒杀之间的冷却时间，0 关闭（CK_ORDER_SECKILL_COOLDOWN）。
	Cooldown time.Duration
	// EnqueueTimeout 是发布 Kafka 时同步等待 ack 的上限，默认 500ms（CK_ORDER_ENQUEUE_TIMEOUT）。
	EnqueueTimeout time.Duration
	// MaxActiveCoupons 是用户同时持有的券总数上限，0 关闭（Java 版默认 15）。
	MaxActiveCoupons int
	// MaxActiveSeckill 是用户同时持有的秒杀券数上限，0 关闭（Java 版默认 5）。
	MaxActiveSeckill int
	// Now 返回当前时间，测试注入；默认 time.Now。
	Now func() time.Time
}

const defaultEnqueueTimeout = 500 * time.Millisecond

// cleanupTimeout 是回滚/收尾这类「不能因调用方 ctx 取消而半途而废」的操作的时限。
const cleanupTimeout = 3 * time.Second

// EnterResult 是 Enter 受理成功的结果。
type EnterResult struct {
	// RequestID 供前端轮询 /seckill/result。
	RequestID string
}

// Service 实现秒杀状态机；并发安全。
type Service struct {
	db       DB
	rdb      redis.Cmdable
	producer kafkax.Producer
	coupon   CouponAPI
	ids      IDSource
	meta     *CouponMeta
	opts     Options
	hooks    []ResultHook

	scripts  *redisx.Scripts
	enter    *redisx.Script
	rollback *redisx.Script
	advance  *redisx.Script
}

// New 创建 Service 并注册 Lua 脚本。hooks 在异步终态之后被调用，见 ResultHook。
func New(d Deps, opts Options, hooks ...ResultHook) (*Service, error) {
	switch {
	case d.DB == nil, d.Redis == nil, d.Producer == nil, d.Coupon == nil, d.IDs == nil:
		return nil, errors.New("seckill: DB, Redis, Producer, Coupon and IDs are all required")
	}
	if opts.EnqueueTimeout <= 0 {
		opts.EnqueueTimeout = defaultEnqueueTimeout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	scripts := redisx.NewScripts(d.Redis)
	return &Service{
		db: d.DB, rdb: d.Redis, producer: d.Producer, coupon: d.Coupon, ids: d.IDs,
		meta: NewCouponMeta(d.Redis, d.Coupon), opts: opts, hooks: hooks,
		scripts:  scripts,
		enter:    scripts.Add("seckill-enter", enterLua),
		rollback: scripts.Add("seckill-rollback", rollbackLua),
		advance:  scripts.Add("seckill-advance", advanceLua),
	}, nil
}

// Meta 返回共享的券元数据读取器，orders 模块下单时复用同一份缓存。
func (s *Service) Meta() *CouponMeta { return s.meta }

// Preload 把脚本预热进 Redis（启动时调用；失败不致命，运行期 NOSCRIPT 会透明重载）。
func (s *Service) Preload(ctx context.Context) error {
	if err := s.scripts.Preload(ctx); err != nil {
		return fmt.Errorf("preload seckill scripts: %w", err)
	}
	return nil
}

func (s *Service) now() time.Time { return s.opts.Now() }

// requestKeys 返回 enter.lua / rollback.lua 共用的 KEYS（顺序固定：stock、占位、冷却、请求哈希、在途集合）。
func requestKeys(requestID string, couponID, userID int64) []string {
	return []string{
		keys.Stock(couponID), keys.SeckillUser(couponID, userID), keys.SeckillCooldown(couponID, userID),
		keys.SeckillRequest(requestID), keys.SeckillInflight(),
	}
}

func ttlSeconds(d time.Duration) int64 { return int64(d / time.Second) }

// Enter 是秒杀入口（设计 §9 步骤 1–4）：校验券 → enter.lua 预扣 → 发布 Kafka → 返回 requestId。
// 失败映射：未开始/已结束/停用/非秒杀券/未预热/冷却/重复/售罄 → 对应业务错误；
// Redis 或 Kafka 故障 → 503 SystemBusy（入队失败时已用 rollback.lua 回补）。
func (s *Service) Enter(ctx context.Context, userID, couponID int64) (EnterResult, error) {
	if userID <= 0 || couponID <= 0 {
		return EnterResult{}, apperr.InvalidRequest.WithMessage("userId 与 couponId 必须为正数")
	}
	info, err := s.meta.Get(ctx, couponID)
	if err != nil {
		return EnterResult{}, err
	}
	now := s.now()
	if err := checkSeckillWindow(info, now); err != nil {
		return EnterResult{}, err
	}

	requestID := logx.NewRequestID()
	code, err := s.enter.Run(ctx, requestKeys(requestID, couponID, userID), requestID, userID, couponID, now.UnixMilli(),
		ttlSeconds(keys.SeckillMarkerTTL), ttlSeconds(keys.SeckillRequestTTL), ceilSeconds(s.opts.Cooldown)).Int64()
	if err != nil {
		// Lua 可能已经生效：请求留在在途集合里，sweeper 会把它收尾，所以这里不需要（也无法安全地）补偿。
		return EnterResult{}, apperr.SystemBusy.WithCause(fmt.Errorf("enter script: %w", err))
	}
	switch code {
	case enterOK:
	case enterSoldOut:
		return EnterResult{}, apperr.OutOfStock
	case enterCoolingDown:
		return EnterResult{}, apperr.CoolingDown
	case enterRepeat:
		return EnterResult{}, apperr.RepeatSeckill
	case enterNotPreheated:
		return EnterResult{}, apperr.NotPreheated
	default:
		return EnterResult{}, apperr.SysError.WithCause(fmt.Errorf("enter script returned unexpected code %d", code))
	}

	payload, err := events.MarshalSeckillOrderCreate(events.NewSeckillOrderCreate(requestID, userID, couponID, now))
	if err != nil {
		s.rollbackEnqueue(ctx, requestID, couponID, userID)
		return EnterResult{}, apperr.SysError.WithCause(err)
	}
	pubCtx, cancel := context.WithTimeout(ctx, s.opts.EnqueueTimeout)
	defer cancel()
	if err := s.producer.Publish(pubCtx, events.TopicSeckillOrderCreate, events.SeckillOrderCreateKey(userID), payload); err != nil {
		// 超时的 Publish 记录可能已送达：消费者读到 FAILED 终态会直接跳过；若它抢在回滚之前已开始处理，
		// 迟到的订单会在置 SUCCESS 时被 CAS 拒绝并回收（见 finalizeSuccess）。
		logx.From(ctx).Error("enqueue failed, rolling back", "request_id", requestID, "coupon_id", couponID, "err", err)
		s.rollbackEnqueue(ctx, requestID, couponID, userID)
		return EnterResult{}, apperr.SystemBusy.WithCause(fmt.Errorf("publish: %w", err))
	}
	return EnterResult{RequestID: requestID}, nil
}

func ceilSeconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64((d + time.Second - 1) / time.Second)
}

// checkSeckillWindow 校验券可被秒杀：启用、是秒杀类型、当前时间在时间窗内（未设置的一端不限）。
func checkSeckillWindow(info internalapi.CouponInfo, now time.Time) error {
	switch {
	case !info.Enabled():
		return apperr.CouponDisabled
	case !info.IsSeckill():
		return apperr.InvalidRequest.WithMessage("该券不是秒杀券，请使用 /api/v1/order/create")
	case info.SeckillStartAt != nil && now.Before(*info.SeckillStartAt):
		return apperr.NotStarted
	case info.SeckillEndAt != nil && now.After(*info.SeckillEndAt):
		return apperr.ActivityEnded
	}
	return nil
}

// rollbackEnqueue 回滚入队失败的请求；回滚本身失败时请求仍在在途集合里，由 sweeper 兜底。
func (s *Service) rollbackEnqueue(ctx context.Context, requestID string, couponID, userID int64) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if _, err := s.runRollback(cctx, requestID, couponID, userID, events.ReasonEnqueueFailed, false); err != nil {
		logx.From(ctx).Error("rollback after enqueue failure failed; sweeper will finalize",
			"request_id", requestID, "coupon_id", couponID, "err", err)
	}
}

// runRollback 执行 rollback.lua。reason 为空表示仅回补不改状态（取消订单）；否则置 FAILED(reason)。
// 返回 rbApplied 等返回码。
func (s *Service) runRollback(ctx context.Context, requestID string, couponID, userID int64, reason events.Reason, keepMarker bool) (int64, error) {
	keep := "0"
	if keepMarker {
		keep = "1"
	}
	code, err := s.rollback.Run(ctx, requestKeys(requestID, couponID, userID), requestID, s.now().UnixMilli(), string(reason), keep,
		ttlSeconds(keys.SeckillRequestTTL)).Int64()
	if err != nil {
		return 0, fmt.Errorf("rollback script: %w", err)
	}
	return code, nil
}

// runAdvance 执行 advance.lua，把请求推进到 PROCESSING 或 SUCCESS；返回 adv* 返回码。
func (s *Service) runAdvance(ctx context.Context, requestID string, userID, couponID int64, target events.Status, orderID int64) (int64, error) {
	code, err := s.advance.Run(ctx, []string{keys.SeckillRequest(requestID), keys.SeckillInflight()}, requestID, string(target),
		s.now().UnixMilli(), ttlSeconds(keys.SeckillRequestTTL), userID, couponID, orderID).Int64()
	if err != nil {
		return 0, fmt.Errorf("advance script: %w", err)
	}
	return code, nil
}

// Restore 归还一张已落库的秒杀订单占用的库存：先调 coupon 的 stock/release（按 requestId 幂等），
// 再经 rollback.lua 做「库存键存在才 INCR + 删占位标记」（rolledBack 标记保证最多回补一次）。
// 用于取消订单；可重复调用，任一步失败返回错误，重试安全。券已被删除时 release 的 404 被容忍。
func (s *Service) Restore(ctx context.Context, requestID string, couponID, userID int64) error {
	if _, err := s.coupon.ReleaseStock(ctx, internalapi.ReleaseRequest{RequestID: requestID, CouponID: couponID}); err != nil {
		if !errors.Is(err, apperr.CouponNotFound) {
			return fmt.Errorf("release stock: %w", err)
		}
		logx.From(ctx).Warn("coupon is gone, skipping DB stock release", "request_id", requestID, "coupon_id", couponID)
	}
	if _, err := s.runRollback(ctx, requestID, couponID, userID, "", false); err != nil {
		return fmt.Errorf("restore redis stock: %w", err)
	}
	return nil
}

// Result 返回请求当前状态（GET /seckill/result）。请求不存在、已过期或不属于 userID 时返回 404/OrderNotFound，
// 不区分两者，避免泄露他人 requestId 是否有效。
func (s *Service) Result(ctx context.Context, userID int64, requestID string) (events.SeckillResult, error) {
	if !events.ValidRequestID(requestID) {
		return events.SeckillResult{}, apperr.InvalidRequest.WithMessage("requestId 格式不正确")
	}
	h, err := s.rdb.HGetAll(ctx, keys.SeckillRequest(requestID)).Result()
	if err != nil {
		return events.SeckillResult{}, apperr.SystemBusy.WithCause(fmt.Errorf("read request hash: %w", err))
	}
	owner, perr := strconv.ParseInt(h[keys.ReqFieldUserID], 10, 64)
	if len(h) == 0 || perr != nil || owner != userID {
		return events.SeckillResult{}, apperr.OrderNotFound.WithMessage("请求不存在或已过期")
	}
	res := events.SeckillResult{Status: events.Status(h[keys.ReqFieldStatus])}
	switch res.Status {
	case events.StatusSuccess:
		if id, err := strconv.ParseInt(h[keys.ReqFieldOrderID], 10, 64); err == nil {
			res.OrderID = id
		}
	case events.StatusFailed:
		res.Reason = events.Reason(h[keys.ReqFieldReason])
		if !res.Reason.Valid() {
			res.Reason = events.ReasonSystemError // FAILED 必须带 reason（前端契约）
		}
	}
	return res, nil
}
