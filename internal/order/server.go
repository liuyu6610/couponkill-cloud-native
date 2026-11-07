package order

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
)

// Deps 是 Server 的外部依赖，由 cmd/order 装配，测试里替换为假实现。
type Deps struct {
	// Config 是已校验的服务配置。
	Config Config
	// DB 是 ord schema 所在的连接池（*pgxpool.Pool）。
	DB seckill.DB
	// Redis 是 Redis 客户端。
	Redis redis.Cmdable
	// Producer 发布建单命令。
	Producer kafkax.Producer
	// Coupon 是 coupon 内部 API 客户端（*internalapi.CouponClient）。
	Coupon seckill.CouponAPI
	// IDs 生成订单与通知 ID（*idgen.Generator）。
	IDs seckill.IDSource
	// Signer 校验网关签名的身份头。
	Signer *authx.GatewaySigner
	// InternalToken 是服务间调用令牌（CK_INTERNAL_TOKEN）。
	InternalToken string
	// Consumer 是建单消费者的 Kafka 配置（Brokers/DLT 或测试用 Source）；Group、Topics、Concurrency、
	// MaxRetries 为空时按 Config 与 events 常量补全。
	Consumer kafkax.ConsumerConfig
	// Ready 是额外的就绪检查（名称 → 检查函数），例如 postgres、redis、kafka。
	Ready map[string]func(context.Context) error
	// Logger 是引擎使用的日志器，默认 slog.Default()。
	Logger *slog.Logger
}

// Option 调整 Server 的组装。
type Option func(*builder)

type builder struct {
	hooks  []seckill.ResultHook
	routes []func(gin.IRouter)
}

// WithResultHook 注册秒杀异步终态的钩子（预约模块借此回写结果）。可多次使用，按注册顺序调用。
func WithResultHook(h seckill.ResultHook) Option {
	return func(b *builder) { b.hooks = append(b.hooks, h) }
}

// WithRoutes 让其他模块（预约、通知）挂路由。
// 传入的 gin.IRouter 挂在根路径上并已经套好 RequireGatewayIdentity：路由用**完整路径**注册
// （如 "/api/v1/order/reservations"，预约、通知模块各自导出了 RoutePrefix 常量），
// 处理器里用 httpx.IdentityFrom(c) 取身份。
func WithRoutes(fn func(gin.IRouter)) Option {
	return func(b *builder) { b.routes = append(b.routes, fn) }
}

// Server 是组装好的 order 服务：HTTP 处理器与后台任务。
type Server struct {
	engine  *httpx.Engine
	seckill *seckill.Service
	orders  *orders.Service
	workers []app.Worker
}

// New 组装服务：构建秒杀状态机与订单域，挂载路由（公开路由在网关身份之后、内部路由在内部令牌之后），
// 并准备消费者与清扫器两个后台任务。
func New(deps Deps, opts ...Option) (*Server, error) {
	var b builder
	for _, o := range opts {
		o(&b)
	}
	if deps.Signer == nil {
		return nil, errors.New("order: Signer is required")
	}
	cfg := deps.Config

	sk, err := seckill.New(seckill.Deps{DB: deps.DB, Redis: deps.Redis, Producer: deps.Producer, Coupon: deps.Coupon, IDs: deps.IDs},
		seckill.Options{
			Cooldown: cfg.SeckillCooldown, EnqueueTimeout: cfg.EnqueueTimeout,
			MaxActiveCoupons: cfg.MaxActiveCoupons, MaxActiveSeckill: cfg.MaxActiveSeckill,
		}, b.hooks...)
	if err != nil {
		return nil, err
	}
	od, err := orders.New(orders.Deps{DB: deps.DB, Coupons: sk.Meta(), Stock: deps.Coupon, Restorer: sk, IDs: deps.IDs},
		orders.Options{
			MaxActiveCoupons: cfg.MaxActiveCoupons, MaxActiveSeckill: cfg.MaxActiveSeckill,
			MaxPageSize: cfg.MaxPageSize, AdminMaxOffset: cfg.AdminMaxOffset,
		})
	if err != nil {
		return nil, err
	}

	eng := httpx.NewEngine(httpx.Options{Service: "order", Logger: deps.Logger})
	for name, check := range deps.Ready {
		eng.Health.Add(name, check)
	}
	identity := httpx.RequireGatewayIdentity(deps.Signer)
	orderGroup := eng.Group("/api/v1/order", identity)
	seckill.NewHandler(sk).Register(orderGroup)
	oh := orders.NewHandler(od)
	oh.Register(orderGroup)
	oh.RegisterUser(eng.Group("/api/v1/user", identity))
	oh.RegisterInternal(eng.Group("", httpx.RequireInternalToken(deps.InternalToken)))
	// 其他模块用各自的完整路径常量注册，所以交给它们根路径上的分组（同样套着网关身份校验）。
	extra := eng.Group("", identity)
	for _, fn := range b.routes {
		fn(extra)
	}

	s := &Server{engine: eng, seckill: sk, orders: od}
	if cfg.ConsumeEnabled {
		consumer := deps.Consumer
		if consumer.Group == "" {
			consumer.Group = events.GroupSeckillOrderCreate
		}
		if len(consumer.Topics) == 0 {
			consumer.Topics = []string{events.TopicSeckillOrderCreate}
		}
		if consumer.Concurrency == 0 {
			consumer.Concurrency = cfg.ConsumeConcurrency
		}
		if consumer.MaxRetries == 0 {
			consumer.MaxRetries = cfg.ConsumeMaxRetries
		}
		s.workers = append(s.workers, app.Worker{Name: "seckill-consumer", Run: func(ctx context.Context) error {
			return sk.Consume(ctx, consumer)
		}})
	}
	if cfg.SweepEnabled {
		sweeper := sk.NewSweeper(seckill.SweeperOptions{Interval: cfg.SweepInterval, Stale: cfg.SweepStale, Batch: cfg.SweepBatch})
		s.workers = append(s.workers, app.Worker{Name: "seckill-sweeper", Run: sweeper.Run})
	}
	return s, nil
}

// Handler 返回 HTTP 处理器（含 /healthz /readyz /metrics）。
func (s *Server) Handler() http.Handler { return s.engine }

// Engine 返回底层引擎，便于 main 挂就绪检查或在测试里直接使用。
func (s *Server) Engine() *httpx.Engine { return s.engine }

// Seckill 返回秒杀状态机，供其他模块（预约到点代抢）复用同一个入口函数：Seckill().Enter(ctx, userID, couponID)。
func (s *Server) Seckill() *seckill.Service { return s.seckill }

// Workers 返回需要交给 app.Run 的后台任务：建单消费者与超时清扫器（按配置开关）。
// 每个 Run 在 ctx 取消时返回；消费者出现不可恢复错误时返回非 nil 错误，app.Run 会据此关闭进程。
func (s *Server) Workers() []app.Worker { return s.workers }
