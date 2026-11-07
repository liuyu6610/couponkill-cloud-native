// Command connector 启动 CouponKill 电商 Connector 服务（端口默认 8085）。
//
// 这里只做装配：加载配置 → 初始化日志 → connector.Build（PG 迁移、Redis、平台适配器、服务）→ app.Run。
// 路由、同步算法、平台适配等全部逻辑都在 internal/connector。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("connector exited with error", "err", err)
		os.Exit(1)
	}
}

// run 装配并运行 connector，返回 nil 表示收到信号后正常退出。
func run() error {
	cfg, err := connector.LoadConfig()
	if err != nil {
		return err
	}
	logger := logx.Setup(logx.Options{Service: "connector", Level: cfg.LogLevel})
	config.Report(logger, &cfg)

	ctx := context.Background()
	rt, err := connector.Build(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("build connector: %w", err)
	}
	return app.Run(ctx, app.Options{
		Name:    "connector",
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: rt.Engine,
		Health:  rt.Engine.Health,
		Workers: rt.Workers,
		OnClose: rt.OnClose,
		Logger:  logger,
		// 手工“同步全部”要逐个调用外部平台，写超时比其它后端服务宽松。
		WriteTimeout: 60 * time.Second,
	})
}
