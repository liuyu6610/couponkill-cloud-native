// Command user 是 CouponKill 的用户服务（注册、登录、登出、资料、压测造用户）。
//
// main 只做进程级装配：加载配置 → 日志 → PostgreSQL（迁移/种子）→ Redis → 路由 → app.Run。
// 业务装配在 internal/user。配置项与端点见 internal/user/README.md。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user"
)

func main() {
	if err := run(); err != nil {
		slog.Error("user 服务退出", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg user.Config
	if err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": user.DefaultPort})); err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}
	logger := logx.Setup(logx.Options{Service: "user", Level: cfg.LogLevel})
	config.Report(logger, &cfg)

	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: cfg.JWTSecret.Reveal(), Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, TTL: cfg.JWTTTL,
	})
	if err != nil {
		return fmt.Errorf("初始化 JWT: %w", err)
	}
	signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal())
	if err != nil {
		return fmt.Errorf("初始化网关签名器: %w", err)
	}
	ids, err := idgen.New(idgen.ResolveWorkerID(cfg.WorkerID))
	if err != nil {
		return fmt.Errorf("初始化 ID 生成器: %w", err)
	}

	ctx := context.Background()
	pool, err := dbx.Open(ctx, cfg.PGDSN.Reveal(), dbx.Options{ApplicationName: "couponkill-user", Logger: logger})
	if err != nil {
		return fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	if err := user.Bootstrap(ctx, pool, cfg, logger); err != nil {
		pool.Close()
		return err
	}
	rdb := redisx.FromConfig(cfg.Redis)

	srv, err := user.New(user.Options{
		Pool: pool, Redis: rdb, Tokens: tokens, Signer: signer,
		InternalToken: cfg.InternalToken.Reveal(), IDs: ids, Logger: logger,
	})
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return err
	}
	srv.Engine.Health.Add("postgres", dbx.Ready(pool))
	srv.Engine.Health.Add("redis", redisx.PingFn(rdb))

	return app.Run(ctx, app.Options{
		Name:    "user",
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: srv.Engine,
		Health:  srv.Engine.Health,
		Logger:  logger,
		OnClose: []func(context.Context) error{
			func(context.Context) error { pool.Close(); return nil },
			func(context.Context) error { return rdb.Close() },
		},
	})
}
