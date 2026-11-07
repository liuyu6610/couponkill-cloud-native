package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 业务上限与默认值。
const (
	// DefaultStockBuckets 是 CK_COUPON_STOCK_BUCKETS 的默认值。
	DefaultStockBuckets = 8
	// MaxStockBuckets 是桶数上限：更多的桶只会让「全桶扫描」变慢。
	MaxStockBuckets = 64
	// MaxStock 是单张券库存的硬上限，防止误灌入极大值（沿用 Java 的同步硬上限）。
	MaxStock = 10_000_000
	// DefaultPreheatConcurrency 是批量预热的默认并发度。
	DefaultPreheatConcurrency = 8
	// availableLimit 是 /available 返回的券数上限。
	availableLimit = 1000
	// DefaultPageSize / MaxPageSize 是 /list 的默认与最大页大小。
	DefaultPageSize = 20
	MaxPageSize     = 100

	lockStripes = 64
)

// Config 是 Service 的可调参数；零值字段取默认值。
type Config struct {
	// StockBuckets 是新建券时库存拆分的桶数，默认 8。已存在的券沿用它们自己的桶数。
	StockBuckets int
	// PreheatConcurrency 是批量预热的并发度，默认 8。
	PreheatConcurrency int
}

func (c Config) withDefaults() Config {
	if c.StockBuckets <= 0 {
		c.StockBuckets = DefaultStockBuckets
	}
	if c.StockBuckets > MaxStockBuckets {
		c.StockBuckets = MaxStockBuckets
	}
	if c.PreheatConcurrency <= 0 {
		c.PreheatConcurrency = DefaultPreheatConcurrency
	}
	return c
}

// Deps 是构造 Service 所需的依赖。
type Deps struct {
	// Repo 是 PostgreSQL 存储（必填）。
	Repo Repo
	// Cache 是 Redis 缓存（必填）。
	Cache Cache
	// Orders 是 order 服务的内部发券客户端（管理员发券需要）。
	Orders internalapi.OrderGranter
	// IDs 是雪花 ID 生成器（必填）。
	IDs IDSource
	// Config 是可调参数。
	Config Config
	// Now 是时钟，默认 time.Now；测试注入。
	Now func() time.Time
	// Logger 默认丢弃日志；main 传入服务 logger。
	Logger *slog.Logger
}

// Service 是 coupon 服务的业务入口，并发安全。
type Service struct {
	repo   Repo
	cache  Cache
	orders internalapi.OrderGranter
	ids    IDSource
	cfg    Config
	now    func() time.Time
	log    *slog.Logger

	// stripes 让同一张券的「读 DB → 写 Redis」刷新串行化，避免两次并发刷新把旧值写在新值后面。
	stripes [lockStripes]sync.Mutex
}

// New 构造 Service；缺少必填依赖返回错误。
func New(d Deps) (*Service, error) {
	if d.Repo == nil || d.Cache == nil || d.IDs == nil {
		return nil, errors.New("coupon service: Repo、Cache、IDs 都是必填依赖")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		repo: d.Repo, cache: d.Cache, orders: d.Orders, ids: d.IDs,
		cfg: d.Config.withDefaults(), now: d.Now, log: d.Logger,
	}, nil
}

// CreateCommand 是创建券的入参（尚未校验）。
type CreateCommand struct {
	// ID 为 0 表示由服务生成。
	ID int64
	// Name 是券名称，1 到 100 个字符。
	Name string
	// Description 是券描述，最多 500 个字符。
	Description string
	// Type 是券类型（1 常驻 / 2 秒杀）。
	Type int
	// FaceValue 是面额的十进制文本（最多两位小数）。
	FaceValue string
	// MinSpend 是使用门槛的十进制文本（最多两位小数）。
	MinSpend string
	// ValidDays 是领取后的有效天数。
	ValidDays int
	// PerUserLimit 是单用户领取上限。
	PerUserLimit int
	// TotalStock 是发行总量（秒杀券即秒杀库存）。
	TotalStock int64
	// SeckillStartAt / SeckillEndAt 是秒杀时间窗，秒杀券必填、常驻券必须为空。
	SeckillStartAt *time.Time
	SeckillEndAt   *time.Time
}

// Create 校验并创建券：在一个事务里写券与各库存桶，提交后刷新 Redis（启用中的秒杀券写计数器）并删除元数据缓存。
// 若 DB 已提交但 Redis 刷新失败，返回 503 并在消息里说明「已保存」，管理员可用预热接口补救。
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (Coupon, error) {
	nc, err := s.validateCreate(cmd)
	if err != nil {
		return Coupon{}, err
	}
	created, err := s.repo.Create(ctx, nc, Distribute(nc.TotalStock, s.cfg.StockBuckets))
	if err != nil {
		return Coupon{}, fmt.Errorf("创建优惠券: %w", err)
	}
	s.log.InfoContext(ctx, "优惠券已创建", "coupon_id", created.ID, "type", created.Type, "total_stock", created.TotalStock)
	if err := s.refresh(ctx, created.ID); err != nil {
		return Coupon{}, err
	}
	return created, nil
}

// Get 返回一张券；includeDisabled 为 false 时停用券按「不存在」处理（C 端不得看到停用券）。
func (s *Service) Get(ctx context.Context, id int64, includeDisabled bool) (Coupon, error) {
	if id <= 0 {
		return Coupon{}, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return Coupon{}, err
	}
	if !includeDisabled && !c.Enabled() {
		return Coupon{}, apperr.CouponNotFound
	}
	return c, nil
}

// Available 返回当前可购买的券：启用且秒杀时间窗尚未结束（含「即将开始」，前端据此展示预约）。
func (s *Service) Available(ctx context.Context) ([]Coupon, error) {
	items, err := s.repo.Available(ctx, s.now(), availableLimit)
	if err != nil {
		return nil, fmt.Errorf("查询可用优惠券: %w", err)
	}
	return items, nil
}

// Page 是分页结果。
type Page struct {
	Items []Coupon
	Total int64
	Page  int
	Size  int
}

// List 分页返回券；includeDisabled 为 false 时只含启用券。page 从 1 开始，size 默认 20、上限 100。
func (s *Service) List(ctx context.Context, page, size int, includeDisabled bool) (Page, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		return Page{}, apperr.InvalidRequest.WithMessage(fmt.Sprintf("size 不能大于 %d", MaxPageSize))
	}
	items, total, err := s.repo.List(ctx, (page-1)*size, size, !includeDisabled)
	if err != nil {
		return Page{}, fmt.Errorf("查询优惠券列表: %w", err)
	}
	return Page{Items: items, Total: total, Page: page, Size: size}, nil
}

// SetSeckillWindow 更新秒杀时间窗：仅秒杀券，start 必须早于 end。成功后删除元数据缓存并刷新计数器，返回更新后的券。
func (s *Service) SetSeckillWindow(ctx context.Context, id int64, start, end time.Time) (Coupon, error) {
	if id <= 0 {
		return Coupon{}, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	if !start.Before(end) {
		return Coupon{}, apperr.InvalidRequest.WithMessage("seckillStartAt 必须早于 seckillEndAt")
	}
	cur, err := s.repo.Get(ctx, id)
	if err != nil {
		return Coupon{}, err
	}
	if !cur.IsSeckill() {
		return Coupon{}, apperr.InvalidRequest.WithMessage("仅秒杀券可设置活动时间窗")
	}
	if err := s.repo.UpdateWindow(ctx, id, start, end); err != nil {
		return Coupon{}, fmt.Errorf("更新秒杀时间窗: %w", err)
	}
	s.log.InfoContext(ctx, "秒杀时间窗已更新", "coupon_id", id, "start", start, "end", end)
	if err := s.refresh(ctx, id); err != nil {
		return Coupon{}, err
	}
	return s.repo.Get(ctx, id)
}

// SetStatus 设置券状态（0 停用 / 1 启用），返回受影响的券数（恒为 1）；券不存在返回 404。
// 停用会删除 Redis 计数器（停用券没有库存闸门），启用秒杀券会重新写入计数器。
func (s *Service) SetStatus(ctx context.Context, id int64, status int) (int, error) {
	if id <= 0 {
		return 0, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	if status != StatusDisabled && status != StatusEnabled {
		return 0, apperr.InvalidRequest.WithMessage("status 只能是 0（停用）或 1（启用）")
	}
	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return 0, fmt.Errorf("更新优惠券状态: %w", err)
	}
	s.log.InfoContext(ctx, "优惠券状态已更新", "coupon_id", id, "status", status)
	if err := s.refresh(ctx, id); err != nil {
		return 0, err
	}
	return 1, nil
}

// Delete 删除券与其库存桶（库存流水保留作审计），返回删除的券数（0 或 1，不存在时幂等返回 0，沿用 Java 语义）。
// 无论是否删到行都会清理 Redis 里的计数器与元数据缓存。
func (s *Service) Delete(ctx context.Context, id int64) (int, error) {
	if id <= 0 {
		return 0, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	n, err := s.repo.Delete(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("删除优惠券: %w", err)
	}
	s.log.InfoContext(ctx, "优惠券已删除", "coupon_id", id, "deleted", n)
	if err := s.refresh(ctx, id); err != nil {
		return 0, err
	}
	return n, nil
}

// Info 返回内部契约 CouponInfo（order 用来校验券并快照到订单），不区分启用状态。
func (s *Service) Info(ctx context.Context, id int64) (internalapi.CouponInfo, error) {
	if id <= 0 {
		return internalapi.CouponInfo{}, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return internalapi.CouponInfo{}, err
	}
	return internalapi.CouponInfo{
		ID: c.ID, Name: c.Name, Type: c.Type, Status: c.Status,
		FaceValue: jsonNumber(c.FaceValue), MinSpend: jsonNumber(c.MinSpend),
		ValidDays: c.ValidDays, PerUserLimit: c.PerUserLimit,
		SeckillStartAt: c.SeckillStartAt, SeckillEndAt: c.SeckillEndAt,
	}, nil
}

// refresh 在 DB 变更提交之后让 Redis 追上 DB：启用中的秒杀券 SET ck:stock，其余情况删除它；
// 同时删除 ck:cpn:meta 让 order 回源。同一张券的刷新串行执行。失败返回带「已保存」说明的 503。
func (s *Service) refresh(ctx context.Context, id int64) error {
	if _, err := s.syncCache(ctx, id, cacheOverwrite); err != nil {
		s.log.ErrorContext(ctx, "DB 已提交但 Redis 刷新失败", "coupon_id", id, "err", err)
		return apperr.New(apperr.CodeSystemBusy, http.StatusServiceUnavailable, "已保存，但库存缓存刷新失败，请调用预热接口重试").WithCause(err)
	}
	return nil
}

type cacheMode int

const (
	// cacheOverwrite 无条件把计数器写成 DB 剩余量。
	cacheOverwrite cacheMode = iota
	// cacheIfAbsent 只在计数器缺失时写入（启动预热：不覆盖在途账本）。
	cacheIfAbsent
)

// syncCache 读取 DB 状态并按 mode 写 Redis，返回所依据的状态。券已不存在时清理两个键。
func (s *Service) syncCache(ctx context.Context, id int64, mode cacheMode) (StockState, error) {
	mu := &s.stripes[uint64(id)%lockStripes]
	mu.Lock()
	defer mu.Unlock()

	st, err := s.repo.StockState(ctx, id)
	if err != nil && !errors.Is(err, apperr.CouponNotFound) {
		return StockState{}, fmt.Errorf("读取券库存状态: %w", err)
	}
	var errs []error
	if err := s.cache.DropMeta(ctx, id); err != nil {
		errs = append(errs, fmt.Errorf("删除元数据缓存: %w", err))
	}
	switch {
	case err != nil || !st.ShouldHaveCounter():
		if derr := s.cache.DropStock(ctx, id); derr != nil {
			errs = append(errs, fmt.Errorf("删除库存计数器: %w", derr))
		}
	case mode == cacheIfAbsent:
		if _, perr := s.cache.PutStockIfAbsent(ctx, id, st.Remaining); perr != nil {
			errs = append(errs, fmt.Errorf("预热库存计数器: %w", perr))
		}
	default:
		if perr := s.cache.PutStock(ctx, id, st.Remaining); perr != nil {
			errs = append(errs, fmt.Errorf("写入库存计数器: %w", perr))
		}
	}
	return st, errors.Join(errs...)
}
