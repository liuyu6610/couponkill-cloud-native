// Package coupon 装配优惠券服务（design §7 cpn、§8 库存键、§10 coupon）：
// 连接 PostgreSQL 与 Redis，迁移 cpn schema，按需写入演示种子，预热库存计数器，然后挂载路由。
//
// 业务规则在 service，PostgreSQL 在 repo，Redis 在 cache，HTTP 在 handler；本包只做装配。
// cmd/coupon/main.go 只调用 New 并把结果交给 app.Run。
package coupon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/cache"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/handler"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// Schema 是本服务独占的 PostgreSQL schema。
const Schema = "cpn"

// DefaultPort 是 coupon 服务的默认 HTTP 端口（design D3）。
const DefaultPort = "8080"

// Config 是 coupon 服务的全部配置（环境变量，前缀 CK_）。
type Config struct {
	config.Common
	config.Postgres
	config.Redis
	config.Internal
	config.GatewaySign
	config.Peers

	// StockBuckets 是新建券时库存拆分的桶数：分散热点行锁。已存在的券沿用自己的桶数。
	StockBuckets int `env:"CK_COUPON_STOCK_BUCKETS" default:"8" min:"1" max:"64"`
	// PreheatConcurrency 是批量预热的并发度。
	PreheatConcurrency int `env:"CK_COUPON_PREHEAT_CONCURRENCY" default:"8" min:"1" max:"64"`
	// InputTZ 是解释不带时区的时间入参（yyyy-MM-dd HH:mm:ss）时使用的 IANA 时区。
	InputTZ string `env:"CK_COUPON_INPUT_TZ" default:"Asia/Shanghai"`
	// SeedDemo 为 true 时启动时写入演示券 1001/1002（幂等）。生产环境禁用。
	SeedDemo bool `env:"CK_SEED_DEMO" default:"false"`
}

// Validate 做 config.Load 无法表达的跨字段校验：生产环境不得开启演示种子，时区名必须可解析。
func (c Config) Validate() error {
	if c.IsProd() && c.SeedDemo {
		return errors.New("CK_ENV=prod 时不得开启 CK_SEED_DEMO（演示数据含已知口令的券与库存）")
	}
	if _, err := time.LoadLocation(c.InputTZ); err != nil {
		return fmt.Errorf("CK_COUPON_INPUT_TZ=%q 无法解析: %w", c.InputTZ, err)
	}
	return nil
}

// Resources 是服务依赖的外部资源。main 通过 Open 创建；测试直接注入（dbxtest 连接池、miniredis、假 order 客户端）。
type Resources struct {
	// Pool 是 PostgreSQL 连接池。
	Pool *pgxpool.Pool
	// Redis 是 Redis 客户端。
	Redis redis.UniversalClient
	// Orders 是 order 内部发券客户端。
	Orders internalapi.OrderGranter
}

// App 是装配完成、可交给 app.Run 的服务。
type App struct {
	// Engine 是已挂载全部路由的 HTTP 引擎（同时提供 Health 与 Registry）。
	Engine *httpx.Engine
	// Service 暴露业务入口，供 main 以外的装配（如 E2E）使用。
	Service *service.Service

	closers []func()
}

// Close 释放 Open 创建的连接池与 Redis 客户端；对注入的资源无副作用。
func (a *App) Close(context.Context) error {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
	return nil
}

// New 打开 PostgreSQL 与 Redis，并执行 Build。失败时已打开的资源会被关闭。
func New(ctx context.Context, cfg Config, log *slog.Logger) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pool, err := dbx.Open(ctx, cfg.PGDSN.Reveal(), dbx.Options{ApplicationName: "couponkill-coupon", Logger: log})
	if err != nil {
		return nil, fmt.Errorf("打开 PostgreSQL: %w", err)
	}
	rdb := redisx.FromConfig(cfg.Redis)
	orders, err := internalapi.NewOrderClient(cfg.OrderURL, cfg.InternalToken.Reveal(), nil,
		internalapi.WithRequestIDSource(logx.RequestID), internalapi.WithLogger(log))
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("创建 order 客户端: %w", err)
	}
	app, err := Build(ctx, cfg, Resources{Pool: pool, Redis: rdb, Orders: orders}, log)
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, err
	}
	app.closers = append(app.closers, pool.Close, func() {
		if err := rdb.Close(); err != nil {
			log.Warn("关闭 Redis 客户端失败", "err", err)
		}
	})
	return app, nil
}

// Build 按设计的启动顺序装配服务：迁移 cpn schema → （CK_SEED_DEMO）演示种子 → 预热库存 → 挂载路由。
// 预热失败（例如 Redis 暂不可用）只记 Error 日志、不阻止启动：管理员可随后调用预热接口，
// 秒杀在计数器缺失期间会得到「未预热」而不是错误的库存。
func Build(ctx context.Context, cfg Config, res Resources, log *slog.Logger) (*App, error) {
	if err := dbx.Migrate(ctx, res.Pool, Schema, migrations.FS); err != nil {
		return nil, fmt.Errorf("迁移 %s schema: %w", Schema, err)
	}
	if cfg.SeedDemo {
		if err := SeedDemo(ctx, res.Pool); err != nil {
			return nil, err
		}
		log.Warn("已写入演示种子数据（CK_SEED_DEMO=true），仅限开发环境")
	}

	gen, err := idgen.New(idgen.ResolveWorkerID(cfg.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("创建 ID 生成器: %w", err)
	}
	svc, err := service.New(service.Deps{
		Repo: repo.New(res.Pool), Cache: cache.New(res.Redis), Orders: res.Orders, IDs: gen, Logger: log,
		Config: service.Config{StockBuckets: cfg.StockBuckets, PreheatConcurrency: cfg.PreheatConcurrency},
	})
	if err != nil {
		return nil, fmt.Errorf("创建 coupon 服务: %w", err)
	}

	// 逐券的失败原因已由 PreheatAll 记录，这里只给出汇总与补救办法。
	if sum, err := svc.PreheatAll(ctx, false); err != nil {
		log.Error("启动预热未全部成功，可稍后调用 POST /api/v1/coupon/preheat-stock 补救",
			"total", sum.Total, "preheated", sum.Preheated, "failed", sum.Failed)
	}

	signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal())
	if err != nil {
		return nil, fmt.Errorf("创建网关签名器: %w", err)
	}
	loc, err := time.LoadLocation(cfg.InputTZ)
	if err != nil {
		return nil, fmt.Errorf("加载时区 %q: %w", cfg.InputTZ, err)
	}
	eng := httpx.NewEngine(httpx.Options{Service: "coupon", Logger: log})
	eng.Health.Add("postgres", dbx.Ready(res.Pool))
	eng.Health.Add("redis", redisx.PingFn(res.Redis))
	if err := handler.Mount(eng, handler.Options{
		Service: svc, Signer: signer, InternalToken: cfg.InternalToken.Reveal(), InputLocation: loc,
	}); err != nil {
		return nil, fmt.Errorf("挂载路由: %w", err)
	}
	return &App{Engine: eng, Service: svc}, nil
}

// SeedDemo 在一个事务里执行迁移包内的 seed_demo.sql（演示券 1001 秒杀、1002 常驻，各 8 桶每桶 200）。
// 文件本身幂等（ON CONFLICT DO NOTHING），可重复执行；只应在 CK_SEED_DEMO=true 时调用。
func SeedDemo(ctx context.Context, pool *pgxpool.Pool) error {
	sqlText, err := migrations.FS.ReadFile(migrations.SeedDemoFile)
	if err != nil {
		return fmt.Errorf("读取演示种子 %s: %w", migrations.SeedDemoFile, err)
	}
	err = dbx.WithTx(ctx, pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, string(sqlText), pgx.QueryExecModeSimpleProtocol)
		return err
	})
	if err != nil {
		return fmt.Errorf("执行演示种子: %w", err)
	}
	return nil
}
