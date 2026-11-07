// Command coupon 启动优惠券服务（默认端口 8080）。配置全部来自 CK_* 环境变量，见 internal/coupon/README.md。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	// 内嵌时区库：distroless 与 Windows 环境没有系统 tzdata，CK_COUPON_INPUT_TZ 依赖它解析 IANA 时区名。
	_ "time/tzdata"

	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("coupon 服务异常退出", "err", err)
		os.Exit(1)
	}
}

// run 加载配置 → 初始化日志 → 装配服务 → 交给 app.Run 管理生命周期。
func run() error {
	var cfg coupon.Config
	if err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": coupon.DefaultPort})); err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}
	logger := logx.Setup(logx.Options{Service: "coupon", Level: cfg.LogLevel})
	config.Report(logger, &cfg)

	ctx := context.Background()
	svc, err := coupon.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("装配 coupon 服务: %w", err)
	}
	return app.Run(ctx, app.Options{
		Name:    "coupon",
		Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler: svc.Engine,
		Health:  svc.Engine.Health,
		Logger:  logger,
		OnClose: []func(context.Context) error{svc.Close},
	})
}
