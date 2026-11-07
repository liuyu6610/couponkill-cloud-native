// Command order 是 order 服务（:8082）的入口：只做装配——加载配置、构建依赖、交给 app.Run。
// 业务逻辑见 internal/order。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/reservation"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/app"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("order service exited with an error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg order.Config
	if err := config.Load(&cfg, config.WithDefaults(map[string]string{"CK_HTTP_PORT": "8082"})); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logx.Setup(logx.Options{Service: "order", Level: cfg.LogLevel})
	config.Report(logger, &cfg)
	ctx := context.Background()

	pool, err := dbx.Open(ctx, cfg.PGDSN.Reveal(), dbx.Options{ApplicationName: "couponkill-order", Logger: logger})
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if err := dbx.Migrate(ctx, pool, "ord", migrations.FS); err != nil {
		return fmt.Errorf("migrate ord schema: %w", err)
	}

	rdb := redisx.FromConfig(cfg.Redis)
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Warn("close redis", "err", err)
		}
	}()

	producer, err := kafkax.NewProducer(kafkax.ProducerConfig{Brokers: cfg.KafkaBrokers, ClientID: "couponkill-order"})
	if err != nil {
		return fmt.Errorf("create kafka producer: %w", err)
	}

	coupon, err := internalapi.NewCouponClient(cfg.CouponURL, cfg.InternalToken.Reveal(), nil,
		internalapi.WithRequestIDSource(logx.RequestID), internalapi.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("create coupon client: %w", err)
	}
	signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal())
	if err != nil {
		return fmt.Errorf("create gateway signer: %w", err)
	}
	ids, err := idgen.New(idgen.ResolveWorkerID(cfg.WorkerID))
	if err != nil {
		return fmt.Errorf("create id generator: %w", err)
	}

	var reservationCfg reservation.Config
	if err := config.Load(&reservationCfg); err != nil {
		return fmt.Errorf("load reservation config: %w", err)
	}
	aux, err := newAux(pool, rdb, ids, coupon, reservationCfg)
	if err != nil {
		return fmt.Errorf("assemble reservation and notification modules: %w", err)
	}

	srv, err := order.New(order.Deps{
		Config: cfg, DB: pool, Redis: rdb, Producer: producer, Coupon: coupon, IDs: ids,
		Signer: signer, InternalToken: cfg.InternalToken.Reveal(),
		Consumer: kafkax.ConsumerConfig{Brokers: cfg.KafkaBrokers, DLT: producer},
		Ready: map[string]func(context.Context) error{
			"postgres": dbx.Ready(pool), "redis": redisx.PingFn(rdb), "kafka": producer.Ping,
		},
		Logger: logger,
	}, aux.options()...)
	if err != nil {
		return fmt.Errorf("assemble order server: %w", err)
	}
	aux.bind(srv.Seckill())
	if err := srv.Seckill().Preload(ctx); err != nil {
		logger.Warn("preloading lua scripts failed; they will be loaded on first use", "err", err)
	}

	return app.Run(ctx, app.Options{
		Name: "order", Addr: fmt.Sprintf(":%d", cfg.HTTPPort), Handler: srv.Handler(), Health: srv.Engine().Health,
		Workers: append(srv.Workers(), aux.worker()),
		OnClose: []func(context.Context) error{func(context.Context) error { return producer.Close() }},
		Logger:  logger,
	})
}
