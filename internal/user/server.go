// Package user 把 user 服务（8081）的 handler、service、repo 装配成一个可运行的 HTTP 服务。
//
// cmd/user/main.go 只做进程级装配（配置、日志、连接、app.Run），业务路由与分层接线都在这里，
// 因此端到端测试可以用同一份装配代码跑真实的 handler+service+仓储+Redis。
package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/handler"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// Schema 是 user 服务独占的 PostgreSQL schema。
const Schema = "usr"

// DefaultPort 是 user 服务的默认 HTTP 端口（设计 D3）。
const DefaultPort = "8081"

// Config 是 user 服务的环境变量配置（设计 §3）。
type Config struct {
	config.Common
	config.Postgres
	config.Redis
	config.JWT
	config.Internal
	config.GatewaySign

	// SeedDemo 是各服务共享的演示种子开关 CK_SEED_DEMO。
	SeedDemo bool `env:"CK_SEED_DEMO" default:"false"`
	// UserSeedDemo 是仅对 user 服务生效的演示种子开关 CK_USER_SEED_DEMO。
	UserSeedDemo bool `env:"CK_USER_SEED_DEMO" default:"false"`
}

// WantsSeedDemo 报告是否请求写入演示用户（任一开关为 true 即可）。
func (c Config) WantsSeedDemo() bool { return c.SeedDemo || c.UserSeedDemo }

// ErrSeedInProd 表示在 CK_ENV=prod 下请求了演示种子：种子口令是公开的测试值，生产绝不允许。
var ErrSeedInProd = errors.New("user: demo seed (CK_SEED_DEMO/CK_USER_SEED_DEMO) is not allowed when CK_ENV=prod")

// Bootstrap 在启动时对 usr schema 执行迁移，并在请求且非生产环境时写入演示用户（幂等）。
func Bootstrap(ctx context.Context, pool *pgxpool.Pool, cfg Config, log *slog.Logger) error {
	if cfg.WantsSeedDemo() && cfg.IsProd() {
		return ErrSeedInProd
	}
	if err := dbx.Migrate(ctx, pool, Schema, migrations.FS); err != nil {
		return fmt.Errorf("migrate schema %s: %w", Schema, err)
	}
	if !cfg.WantsSeedDemo() {
		return nil
	}
	if err := repo.New(pool).SeedDemo(ctx); err != nil {
		return fmt.Errorf("seed demo users: %w", err)
	}
	log.Warn("已写入演示用户 demo/admin（口令为公开测试值，仅限开发环境）")
	return nil
}

// Options 是 New 的依赖。
type Options struct {
	// Pool 是已迁移的 PostgreSQL 连接池。
	Pool *pgxpool.Pool
	// Redis 是会话存储（ck:sess:{jti}）使用的客户端。
	Redis redis.Cmdable
	// Tokens 签发与校验 JWT。
	Tokens *authx.Tokens
	// Signer 校验网关身份签名。
	Signer *authx.GatewaySigner
	// InternalToken 是内部端点令牌。
	InternalToken string
	// IDs 生成用户 ID（雪花）。
	IDs service.IDGenerator
	// Logger 默认 slog.Default()。
	Logger *slog.Logger
}

// Server 是装配好的 user 服务。
type Server struct {
	// Engine 是 HTTP 引擎（已挂 /healthz /readyz /metrics 与全部业务路由）。
	Engine *httpx.Engine
}

// New 把仓储、service、handler 接成一个 HTTP 引擎。就绪检查（PG/Redis）由调用方通过 Engine.Health 添加。
func New(o Options) (*Server, error) {
	svc, err := service.New(service.Deps{
		Repo:     repo.New(o.Pool),
		Sessions: redisx.NewSessionStore(o.Redis),
		Tokens:   o.Tokens,
		IDs:      o.IDs,
		Logger:   o.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("build user service: %w", err)
	}
	eng := httpx.NewEngine(httpx.Options{Service: "user", Logger: o.Logger})
	handler.Mount(eng.Engine, handler.Options{Service: svc, Signer: o.Signer, InternalToken: o.InternalToken})
	return &Server{Engine: eng}, nil
}
