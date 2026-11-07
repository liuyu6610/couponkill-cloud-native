// Command gateway 启动 CouponKill API 网关（端口默认 8088）。
//
// 这里只做装配：加载配置 → 初始化日志 → 连接 Redis（仅在启用会话吊销检查时）→ 构建网关 → app.Run。
// 路由、鉴权、限流、熔断等全部逻辑都在 internal/gateway。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/gateway"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited with error", "err", err)
		os.Exit(1)
	}
}

// run 装配并运行网关，返回 nil 表示收到信号后正常退出。
func run() error {
	cfg, err := gateway.LoadConfig()
	if err != nil {
		return err
	}
	logger := logx.Setup(logx.Options{Service: "gateway", Level: cfg.LogLevel})
	config.Report(logger, &cfg)

	deps := gateway.Deps{Config: cfg, Logger: logger}
	var onClose []func(context.Context) error
	if cfg.RevocationEnabled {
		rdb := redisx.FromConfig(cfg.Redis)
		deps.Revoker = redisx.NewSessionStore(rdb)
		deps.Ping = redisx.PingFn(rdb)
		onClose = append(onClose, func(context.Context) error { return rdb.Close() })
	} else {
		logger.Warn("session revocation check is disabled: logged-out tokens stay valid until they expire")
	}

	gw, err := gateway.New(deps)
	if err != nil {
		return fmt.Errorf("build gateway: %w", err)
	}
	onClose = append(onClose, func(context.Context) error { gw.Close(); return nil })

	return app.Run(context.Background(), app.Options{
		Name:    "gateway",
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: gw.Handler(),
		Health:  gw.Engine().Health,
		OnClose: onClose,
		Logger:  logger,
		// 网关要透传较大的响应体，写超时比后端服务宽松；上游响应头的等待时间由网关自己限制。
		WriteTimeout: 60 * time.Second,
	})
}
