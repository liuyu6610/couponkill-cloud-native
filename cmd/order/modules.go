package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/reservation"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// auxModules 把预约（reservation）与站内通知（notification）接进 order 服务。
//
// 这两个模块与秒杀模块互不引用，只通过下面几个小适配器相连：
//   - 预约到点代抢：reservation.Seckiller ← seckill.Service.Enter（lateSeckiller 延迟绑定）；
//   - 秒杀终态回写预约：seckill.ResultHook → reservation.Service.OnSeckillResult（resultHook）；
//   - 结果事件丢失的兜底：reservation.ResultReader ← Redis 请求哈希 ck:sk:req:{id}（redisResultReader）；
//   - 取券事实：reservation.CouponLookup ← internalapi.CouponClient（couponLookup）。
type auxModules struct {
	reservations  *reservation.Service
	scheduler     *reservation.Scheduler
	notifications *notification.Service
	late          *lateSeckiller
}

// newAux 构建预约与通知模块。scheduler 此时还拿不到秒杀入口（order.New 之后才有），
// 所以先给它一个延迟绑定的 Seckiller，order.New 完成后由 bind 补上。
func newAux(pool *pgxpool.Pool, rdb redis.Cmdable, ids reservation.IDGenerator, coupon couponGetter, cfg reservation.Config) (*auxModules, error) {
	late := &lateSeckiller{}
	svc := reservation.NewService(pool, ids, couponLookup{c: coupon})
	sched, err := reservation.NewScheduler(svc, late, cfg, reservation.WithResultReader(redisResultReader{rdb: rdb}))
	if err != nil {
		return nil, fmt.Errorf("create reservation scheduler: %w", err)
	}
	return &auxModules{reservations: svc, scheduler: sched, notifications: notification.NewService(pool), late: late}, nil
}

// options 返回交给 order.New 的选项：秒杀终态钩子与两组路由。
func (a *auxModules) options() []order.Option {
	return []order.Option{
		order.WithResultHook(resultHook{svc: a.reservations}),
		order.WithRoutes(func(r gin.IRouter) {
			reservation.RegisterRoutes(r, a.reservations)
			notification.RegisterRoutes(r, a.notifications)
		}),
	}
}

// bind 在 order.New 之后把秒杀入口交给预约调度器。
func (a *auxModules) bind(sk *seckill.Service) { a.late.Bind(sk) }

// worker 是预约到点调度任务，交给 app.Run 与其他后台任务一起管理。
func (a *auxModules) worker() app.Worker {
	return app.Worker{Name: "reservation-scheduler", Run: a.scheduler.Run}
}

// resultSink 是 reservation.Service 中接收秒杀终态的那一个方法。
type resultSink interface {
	OnSeckillResult(ctx context.Context, ev reservation.ResultEvent) error
}

// resultHook 把 order-core 的秒杀终态翻译成预约模块的结果事件。
type resultHook struct{ svc resultSink }

// OnSeckillResult 实现 seckill.ResultHook；错误只会被 order-core 记日志，不会让建单消息重试。
func (h resultHook) OnSeckillResult(ctx context.Context, ev seckill.ResultEvent) error {
	return h.svc.OnSeckillResult(ctx, reservation.ResultEvent{
		RequestID: ev.RequestID, UserID: ev.UserID, CouponID: ev.CouponID,
		Success: ev.Success, OrderID: ev.OrderID, Reason: string(ev.Reason),
	})
}

// seckillEnterer 是 seckill.Service.Enter 的最小接口。
type seckillEnterer interface {
	Enter(ctx context.Context, userID, couponID int64) (seckill.EnterResult, error)
}

// lateSeckiller 是延迟绑定的 reservation.Seckiller：绑定之前返回可重试的 SystemBusy，调度器会退避重试，
// 不会把预约判成终态失败。
type lateSeckiller struct {
	mu sync.RWMutex
	sk seckillEnterer
}

// Bind 绑定真正的秒杀入口。
func (l *lateSeckiller) Bind(sk seckillEnterer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sk = sk
}

// Enter 实现 reservation.Seckiller：业务错误（RepeatSeckill、ActivityEnded 等）原样透传，调度器按语义分类。
func (l *lateSeckiller) Enter(ctx context.Context, userID, couponID int64) (string, error) {
	l.mu.RLock()
	sk := l.sk
	l.mu.RUnlock()
	if sk == nil {
		return "", apperr.SystemBusy.WithMessage("秒杀入口尚未就绪")
	}
	res, err := sk.Enter(ctx, userID, couponID)
	if err != nil {
		return "", err
	}
	return res.RequestID, nil
}

// couponGetter 是 internalapi.CouponClient.GetCoupon 的最小接口。
type couponGetter interface {
	GetCoupon(ctx context.Context, id int64) (internalapi.CouponInfo, error)
}

// couponLookup 把 coupon 服务的券信息适配成预约模块需要的券事实。
type couponLookup struct{ c couponGetter }

// Coupon 实现 reservation.CouponLookup：券不存在时 coupon 服务返回的 apperr.CouponNotFound 原样透传。
func (l couponLookup) Coupon(ctx context.Context, couponID int64) (reservation.Coupon, error) {
	info, err := l.c.GetCoupon(ctx, couponID)
	if err != nil {
		return reservation.Coupon{}, err
	}
	return reservation.Coupon{
		ID: info.ID, Type: info.Type, Status: info.Status,
		SeckillStartAt: info.SeckillStartAt, SeckillEndAt: info.SeckillEndAt,
	}, nil
}

// redisResultReader 是 reservation.ResultReader：读 order-core 写的请求哈希，兜底结果事件丢失
// （例如建单消息进了死信）而让预约永远停在 QUEUED 的情况。
type redisResultReader struct{ rdb redis.Cmdable }

// Result 只在请求到达终态（SUCCESS/FAILED）时返回 found=true；处理中或不存在都返回 found=false，
// Redis 故障返回错误，调用方下一轮再查。
func (r redisResultReader) Result(ctx context.Context, requestID string) (reservation.ResultEvent, bool, error) {
	m, err := r.rdb.HGetAll(ctx, keys.SeckillRequest(requestID)).Result()
	if err != nil {
		return reservation.ResultEvent{}, false, fmt.Errorf("read seckill request %s: %w", requestID, err)
	}
	status := events.Status(m[keys.ReqFieldStatus])
	if status != events.StatusSuccess && status != events.StatusFailed {
		return reservation.ResultEvent{}, false, nil
	}
	ev := reservation.ResultEvent{
		RequestID: requestID,
		UserID:    parseID(m[keys.ReqFieldUserID]),
		CouponID:  parseID(m[keys.ReqFieldCouponID]),
		Success:   status == events.StatusSuccess,
	}
	if ev.Success {
		ev.OrderID = parseID(m[keys.ReqFieldOrderID])
	} else {
		ev.Reason = m[keys.ReqFieldReason]
	}
	return ev, true, nil
}

// parseID 解析请求哈希里十进制字符串形式的 ID；缺失或非法返回 0（预约模块把 0 当作「未知」）。
func parseID(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
