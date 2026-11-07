package orders

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// CouponSource 读取券详情（带缓存）；*seckill.CouponMeta 满足它。
type CouponSource interface {
	Get(ctx context.Context, couponID int64) (internalapi.CouponInfo, error)
}

// StockAPI 是常驻券下单需要的库存接口（*internalapi.CouponClient 满足它）。
type StockAPI interface {
	internalapi.StockDeductor
	internalapi.StockReleaser
}

// StockRestorer 归还一张已落库的秒杀订单占用的库存（DB 桶 + Redis 计数 + 占位标记），幂等；
// *seckill.Service 满足它。接口在使用方定义，因此 orders 不依赖 seckill 包。
type StockRestorer interface {
	Restore(ctx context.Context, requestID string, couponID, userID int64) error
}

// IDSource 生成订单 ID（*idgen.Generator 满足它）。
type IDSource interface {
	Next() int64
}

// DB 是 Service 需要的数据库能力（*pgxpool.Pool 满足它）。
type DB interface {
	dbx.TxBeginner
	Querier
}

// Deps 是 Service 的外部依赖。
type Deps struct {
	// DB 是 ord schema 所在的连接池。
	DB DB
	// Coupons 读取券详情。
	Coupons CouponSource
	// Stock 扣减/归还常驻券库存。
	Stock StockAPI
	// Restorer 归还秒杀订单的库存。
	Restorer StockRestorer
	// IDs 生成订单 ID。
	IDs IDSource
}

// Options 是 Service 的可调参数；零值字段取默认值。
type Options struct {
	// MaxActiveCoupons 是用户同时持有的券总数上限，0 关闭（Java 版默认 15）。
	MaxActiveCoupons int
	// MaxActiveSeckill 是用户同时持有的秒杀券数上限，0 关闭（Java 版默认 5）；常驻券下单只检查总数。
	MaxActiveSeckill int
	// MaxPageSize 是列表单页最大条数，默认 100（超出被截断）。
	MaxPageSize int
	// AdminMaxOffset 是管理端列表最大偏移，默认 10000（超出返回 400，提示缩小范围）。
	AdminMaxOffset int
	// Now 返回当前时间，测试注入；默认 time.Now。
	Now func() time.Time
}

const (
	defaultMaxPageSize    = 100
	defaultAdminMaxOffset = 10000
	defaultPageSize       = 10
	releaseTimeout        = 3 * time.Second
)

// Service 是订单域的业务规则；并发安全。
type Service struct {
	db       DB
	coupons  CouponSource
	stock    StockAPI
	restorer StockRestorer
	ids      IDSource
	opts     Options
}

// New 创建 Service。
func New(d Deps, opts Options) (*Service, error) {
	if d.DB == nil || d.Coupons == nil || d.Stock == nil || d.Restorer == nil || d.IDs == nil {
		return nil, errors.New("orders: DB, Coupons, Stock, Restorer and IDs are all required")
	}
	if opts.MaxPageSize <= 0 {
		opts.MaxPageSize = defaultMaxPageSize
	}
	if opts.AdminMaxOffset <= 0 {
		opts.AdminMaxOffset = defaultAdminMaxOffset
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{db: d.DB, coupons: d.Coupons, stock: d.Stock, restorer: d.Restorer, ids: d.IDs, opts: opts}, nil
}

func (s *Service) now() time.Time { return s.opts.Now() }

// Create 为用户领取一张常驻券（POST /create）：校验 → 持有上限 → 扣库存 → 落单；落单失败则归还库存（saga）。
// 秒杀券返回 10012（请走秒杀接口）；用户已持有该券（含并发竞争命中部分唯一索引）返回 10010。
//
// 每次调用使用新的 requestId 作为库存账本与订单的幂等键。库存扣减遇到 transient 失败时结果未知，
// 同样会补一次归还（账本对未扣减的 requestId 写墓碑，不会造成负库存）。
func (s *Service) Create(ctx context.Context, userID, couponID int64) (Order, error) {
	info, err := s.coupons.Get(ctx, couponID)
	if err != nil {
		return Order{}, err
	}
	switch {
	case !info.Enabled():
		return Order{}, apperr.CouponDisabled
	case info.IsSeckill():
		return Order{}, apperr.SeckillUseDedicatedAPI
	}
	if held, err := HasActive(ctx, s.db, userID, couponID); err != nil {
		return Order{}, err
	} else if held {
		return Order{}, apperr.RepeatSeckill
	}
	if err := s.checkCaps(ctx, userID); err != nil {
		return Order{}, err
	}

	requestID := logx.NewRequestID()
	ctx = logx.With(ctx, "request_id", requestID, "user_id", userID, "coupon_id", couponID)
	if _, err := s.stock.DeductStock(ctx, internalapi.DeductRequest{RequestID: requestID, CouponID: couponID, UserID: userID}); err != nil {
		if internalapi.IsTransient(err) {
			s.releaseQuietly(ctx, requestID, couponID)
		}
		return Order{}, err
	}

	now := s.now()
	id, _, err := Insert(ctx, s.db, NewOrder{
		ID: s.ids.Next(), UserID: userID, CouponID: couponID, RequestID: requestID,
		CouponName: info.Name, CouponType: info.Type, FaceValue: info.FaceValue.String(), MinSpend: info.MinSpend.String(),
		Source: SourceNormal, Now: now, ExpireAt: now.AddDate(0, 0, max(info.ValidDays, 1)),
	})
	if err != nil {
		s.releaseQuietly(ctx, requestID, couponID)
		if dbx.IsUniqueViolation(err, "") {
			return Order{}, apperr.RepeatSeckill
		}
		return Order{}, apperr.SysError.WithCause(err)
	}
	order, found, err := FindByID(ctx, s.db, userID, id)
	if err != nil || !found {
		// 订单已经落库：不能再归还库存。读不到只影响返回体，用已知字段兜底。
		logx.From(ctx).Warn("created order could not be re-read", "order_id", id, "err", err)
		return Order{ID: id, UserID: userID, CouponID: couponID, RequestID: requestID, Status: StatusCreated, Source: SourceNormal,
			CouponName: info.Name, CouponType: info.Type, FaceValue: info.FaceValue, MinSpend: info.MinSpend, GetTime: now, ExpireTime: now.AddDate(0, 0, max(info.ValidDays, 1)),
			CreatedAt: now, UpdatedAt: now}, nil
	}
	return order, nil
}

// checkCaps 复刻 Java 版的持有上限：总数与（秒杀）数量；常驻券下单只受总数约束。
func (s *Service) checkCaps(ctx context.Context, userID int64) error {
	if s.opts.MaxActiveCoupons <= 0 {
		return nil
	}
	c, err := CountsByUser(ctx, s.db, userID)
	if err != nil {
		return err
	}
	if c.Total >= s.opts.MaxActiveCoupons {
		return apperr.CouponLimitExceeded
	}
	return nil
}

// releaseQuietly 在 saga 失败路径上归还库存：用与请求 ctx 解耦的短超时，失败只记日志（需要对账）。
func (s *Service) releaseQuietly(ctx context.Context, requestID string, couponID int64) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if _, err := s.stock.ReleaseStock(rctx, internalapi.ReleaseRequest{RequestID: requestID, CouponID: couponID}); err != nil {
		logx.From(ctx).Error("stock release after failed order creation failed; needs reconciliation",
			"request_id", requestID, "coupon_id", couponID, "err", err)
	}
}

// Cancel 取消订单（POST /cancel）：事务内只做 status 1→4，成功后归还库存。
//
// 返回 true 表示订单现在处于已取消状态（无论本次调用还是此前的调用取消的）；已使用/已过期的订单返回 false。
// 取消别人的订单与订单不存在无法区分，一律 10011。并发取消只有一个能赢得 1→4，库存只会被归还一次：
// 归还本身是幂等的（coupon 账本 + rollback.lua 的 rolledBack 标记），所以对已取消订单再次调用 Cancel 会重新执行归还，
// 这让「取消已提交、归还失败」的情形可以靠客户端重试自愈。归还失败时返回错误（订单仍然是已取消）。
func (s *Service) Cancel(ctx context.Context, userID, orderID int64) (bool, error) {
	order, found, err := FindByID(ctx, s.db, userID, orderID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, apperr.OrderNotFound
	}
	ctx = logx.With(ctx, "order_id", orderID, "user_id", userID, "coupon_id", order.CouponID)

	if order.Status == StatusCreated {
		won, err := Cancel(ctx, s.db, userID, orderID, s.now())
		if err != nil {
			return false, err
		}
		if !won {
			// 并发的另一个取消/核销抢先了：重读决定。
			order, found, err = FindByID(ctx, s.db, userID, orderID)
			if err != nil {
				return false, err
			}
			if !found {
				return false, apperr.OrderNotFound
			}
		} else {
			order.Status = StatusCancelled
		}
	}
	if order.Status != StatusCancelled {
		return false, nil
	}
	if err := s.restore(ctx, order); err != nil {
		return false, err
	}
	return true, nil
}

// restore 归还已取消订单占用的库存：秒杀订单走 Restorer（含 Redis 计数与占位），其余订单只归还 DB 桶。
func (s *Service) restore(ctx context.Context, order Order) error {
	if order.Source == SourceSeckill {
		if err := s.restorer.Restore(ctx, order.RequestID, order.CouponID, order.UserID); err != nil {
			return fmt.Errorf("restore stock of order %d: %w", order.ID, err)
		}
		return nil
	}
	_, err := s.stock.ReleaseStock(ctx, internalapi.ReleaseRequest{RequestID: order.RequestID, CouponID: order.CouponID})
	if err != nil && !errors.Is(err, apperr.CouponNotFound) {
		return fmt.Errorf("release stock of order %d: %w", order.ID, err)
	}
	return nil
}

// Holds 判断用户是否持有该券的未取消订单（GET /check/{couponId}）。
func (s *Service) Holds(ctx context.Context, userID, couponID int64) (bool, error) {
	return HasActive(ctx, s.db, userID, couponID)
}

// Counts 返回用户持有的券统计（GET /api/v1/user/coupon/count）。
func (s *Service) Counts(ctx context.Context, userID int64) (Counts, error) {
	return CountsByUser(ctx, s.db, userID)
}

// PageRequest 是分页入参；Page 从 1 开始，非法值回落到默认。
type PageRequest struct {
	// Page 是页码（从 1 开始）。
	Page int
	// Size 是页大小。
	Size int
}

// normalize 回落非法值并截断页大小，返回规范化后的页码、页大小与偏移（int64 防溢出）。
func (s *Service) normalize(p PageRequest) (page, size int, offset int64) {
	page, size = p.Page, p.Size
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > s.opts.MaxPageSize {
		size = s.opts.MaxPageSize
	}
	return page, size, int64(page-1) * int64(size)
}

// Page 是分页结果。
type Page struct {
	// Items 是当前页的订单。
	Items []Order
	// Total 是总数。
	Total int64
	// Page 与 Size 是规范化后的页码与页大小。
	Page int
	Size int
}

// ListByUser 返回用户订单分页（带 user_id，只扫一个分区）。
func (s *Service) ListByUser(ctx context.Context, userID int64, p PageRequest) (Page, error) {
	page, size, offset := s.normalize(p)
	if offset > math.MaxInt32 {
		return Page{}, apperr.InvalidRequest.WithMessage("页码过大")
	}
	items, total, err := ListByUser(ctx, s.db, userID, int(offset), size)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, Total: total, Page: page, Size: size}, nil
}

// ListAll 是管理端分页：页大小与最大偏移封顶，超出偏移返回 400 并提示缩小范围。
func (s *Service) ListAll(ctx context.Context, f AdminFilter, p PageRequest) (Page, error) {
	page, size, offset := s.normalize(p)
	if offset > int64(s.opts.AdminMaxOffset) {
		return Page{}, apperr.InvalidRequest.WithMessage(
			fmt.Sprintf("管理台订单列表最多翻到第 %d 条，请缩小时间范围或增加过滤条件后再查", s.opts.AdminMaxOffset))
	}
	items, total, err := ListAll(ctx, s.db, f, int(offset), size)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, Total: total, Page: page, Size: size}, nil
}

// Grant 给用户直接建一张订单（管理员发券，POST /internal/v1/orders/grant）：不触碰库存（调用方 coupon 已扣），
// 按 (user_id, request_id) 幂等——重放返回同一订单并置 AlreadyGranted；用户已持有该券返回 10010。
func (s *Service) Grant(ctx context.Context, req internalapi.GrantRequest) (internalapi.GrantResponse, error) {
	if err := req.Validate(); err != nil {
		return internalapi.GrantResponse{}, err
	}
	if prior, found, err := FindByRequest(ctx, s.db, req.UserID, req.RequestID); err != nil {
		return internalapi.GrantResponse{}, err
	} else if found {
		return internalapi.GrantResponse{OrderID: prior.ID, AlreadyGranted: true}, nil
	}
	info, err := s.coupons.Get(ctx, req.CouponID)
	if err != nil {
		return internalapi.GrantResponse{}, err
	}
	now := s.now()
	id, inserted, err := Insert(ctx, s.db, NewOrder{
		ID: s.ids.Next(), UserID: req.UserID, CouponID: req.CouponID, RequestID: req.RequestID,
		CouponName: info.Name, CouponType: info.Type, FaceValue: info.FaceValue.String(), MinSpend: info.MinSpend.String(),
		Source: SourceAdmin, Now: now, ExpireAt: now.AddDate(0, 0, max(info.ValidDays, 1)),
	})
	switch {
	case err == nil && inserted:
		return internalapi.GrantResponse{OrderID: id}, nil
	case err == nil:
		// 并发的同一 requestId 抢先落了单。
		prior, found, ferr := FindByRequest(ctx, s.db, req.UserID, req.RequestID)
		if ferr != nil || !found {
			return internalapi.GrantResponse{}, apperr.SysError.WithCause(fmt.Errorf("grant replay lookup: found=%v err=%v", found, ferr))
		}
		return internalapi.GrantResponse{OrderID: prior.ID, AlreadyGranted: true}, nil
	case dbx.IsUniqueViolation(err, ""):
		// 并发的同一 requestId 可能先撞上部分唯一索引而不是仲裁索引：能按请求号找到订单就是重放，否则才是已持有。
		if prior, found, ferr := FindByRequest(ctx, s.db, req.UserID, req.RequestID); ferr == nil && found {
			return internalapi.GrantResponse{OrderID: prior.ID, AlreadyGranted: true}, nil
		}
		return internalapi.GrantResponse{}, apperr.RepeatSeckill
	default:
		return internalapi.GrantResponse{}, apperr.SysError.WithCause(err)
	}
}
