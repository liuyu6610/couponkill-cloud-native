package connector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

// Runtime 是装配完成、可以交给 app.Run 的服务：引擎、后台任务与关闭钩子。
type Runtime struct {
	// Engine 是 HTTP 处理器（同时提供 Health 给 app.Run）。
	Engine *httpx.Engine
	// Workers 是后台任务（定时库存同步，可被配置关闭）。
	Workers []app.Worker
	// OnClose 是关闭钩子（连接池、Redis），按 app.Run 约定逆序执行。
	OnClose []func(ctx context.Context) error
}

// Build 按配置装配整个服务：连接 PG 并在 advisory lock 下迁移 cnx schema → Redis → 平台适配器与注册表
// → 各 service → HTTP 引擎与定时任务。任何一步失败都会释放已打开的资源并返回错误。
func Build(ctx context.Context, cfg Config, log *slog.Logger) (rt *Runtime, err error) {
	pool, err := dbx.Open(ctx, cfg.PGDSN.Reveal(), dbx.Options{ApplicationName: "couponkill-connector", Logger: log})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	rdb := redisx.FromConfig(cfg.Redis)
	defer func() {
		if err != nil {
			_ = rdb.Close()
			pool.Close()
		}
	}()
	if err = dbx.Migrate(ctx, pool, "cnx", migrations.FS); err != nil {
		return nil, fmt.Errorf("migrate cnx schema: %w", err)
	}
	gen, err := idgen.New(idgen.ResolveWorkerID(cfg.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("init id generator: %w", err)
	}
	signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal())
	if err != nil {
		return nil, fmt.Errorf("init gateway signer: %w", err)
	}
	couponClient, err := internalapi.NewCouponClient(cfg.CouponURL, cfg.InternalToken.Reveal(), nil)
	if err != nil {
		return nil, fmt.Errorf("init coupon client: %w", err)
	}

	jd := platforms.NewJD(platforms.JDConfig{
		ServerURL: cfg.JDServerURL, AppKey: cfg.JDAppKey, AppSecret: cfg.JDAppSecret, AccessToken: cfg.JDAccessToken,
		DefaultArea: cfg.JDDefaultArea, Caller: callerConfig(cfg),
	})
	reg, err := platforms.NewRegistry(platforms.RegistryConfig{HealthTTL: cfg.HealthTTL},
		platforms.NewMock(platforms.MockConfig{DefaultStock: cfg.MockStock}),
		jd,
		platforms.NewTB(platforms.TBConfig{ServerURL: cfg.TBServerURL, AppKey: cfg.TBAppKey, AppSecret: cfg.TBAppSecret, Session: cfg.TBSession, Caller: callerConfig(cfg)}),
		platforms.NewPDD(platforms.PDDConfig{ServerURL: cfg.PDDServerURL, AppKey: cfg.PDDAppKey, AppSecret: cfg.PDDAppSecret, AccessToken: cfg.PDDAccessToken, Caller: callerConfig(cfg)}),
	)
	if err != nil {
		return nil, fmt.Errorf("build platform registry: %w", err)
	}

	store := repo.NewStore(pool, gen)
	syncSvc := service.NewSyncService(store, reg, couponClient, service.NewRedisLocker(rdb, cfg.SyncLockTTL),
		service.SyncConfig{Budget: cfg.SyncBudget, LockTTL: cfg.SyncLockTTL})
	eng := NewEngine(Deps{
		Bindings: service.NewBindingService(store),
		Prices:   service.NewPriceService(store, reg, service.PriceConfig{CacheTTL: cfg.CompareCacheTTL}),
		Probe:    service.NewProbeService(reg),
		Sync:     syncSvc,
		Signer:   signer,
		JD:       jd,
		Logger:   log,
	})
	eng.Health.Add("postgres", dbx.Ready(pool))
	eng.Health.Add("redis", redisx.PingFn(rdb))

	rt = &Runtime{
		Engine: eng,
		OnClose: []func(context.Context) error{
			func(context.Context) error { pool.Close(); return nil },
			func(context.Context) error { return rdb.Close() },
		},
	}
	if cfg.SyncEnabled {
		job := service.NewSyncJob(syncSvc, cfg.SyncInterval)
		rt.Workers = append(rt.Workers, app.Worker{Name: "stock-sync", Run: job.Run})
	} else {
		log.Warn("scheduled stock sync disabled (CK_CONNECTOR_SYNC_ENABLED=false); manual sync endpoints still work")
	}
	return rt, nil
}

// callerConfig 把统一的超时/重试/熔断配置翻译成每个平台各自的 CallerConfig（熔断器每平台独立）。
func callerConfig(cfg Config) platforms.CallerConfig {
	retries := cfg.CallRetries
	if retries == 0 {
		retries = -1 // Caller 里 0 表示默认值，负数才是“不重试”
	}
	return platforms.CallerConfig{
		Retries: retries,
		Breaker: limiter.BreakerConfig{FailureThreshold: cfg.BreakerFailures, OpenTimeout: cfg.BreakerOpenFor},
	}
}
