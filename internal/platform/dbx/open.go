// Package dbx 封装 PostgreSQL 访问的公共基础设施：连接池、事务助手、错误判定与迁移执行器。
//
// 约定（见 docs/GO-REWRITE-DESIGN.md §2/§7）：单库 couponkill，每个服务一个 schema，
// 服务只访问自己的 schema；迁移 SQL 以 embed.FS 内嵌在各服务的 migrations 包里，服务启动时
// 在 advisory lock 保护下通过 Migrate 执行。
package dbx

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 连接池默认值。
const (
	defaultMaxConns          = 20
	defaultMinConns          = 2
	defaultMaxConnLifetime   = 30 * time.Minute
	defaultMaxConnIdleTime   = 5 * time.Minute
	defaultHealthCheckPeriod = 30 * time.Second
	defaultConnectTimeout    = 5 * time.Second
	defaultStatementTimeout  = 30 * time.Second
	defaultSlowQuery         = 200 * time.Millisecond
	slowQuerySQLMaxLen       = 300
)

// Options 是 Open 的可选项，零值字段取默认值。DSN 里的 pool_* 参数会被忽略，统一由 Options 控制。
type Options struct {
	// MaxConns 连接池上限，默认 20。
	MaxConns int32
	// MinConns 常驻最小连接数，默认 2（不得超过 MaxConns）。
	MinConns int32
	// MaxConnLifetime 连接最长存活时间，默认 30 分钟（便于滚动更换后端/DNS）。
	MaxConnLifetime time.Duration
	// MaxConnIdleTime 空闲连接回收时间，默认 5 分钟。
	MaxConnIdleTime time.Duration
	// HealthCheckPeriod 后台健康检查周期，默认 30 秒。
	HealthCheckPeriod time.Duration
	// ConnectTimeout 单次建连与首次 Ping 的超时，默认 5 秒。
	ConnectTimeout time.Duration
	// StatementTimeout 服务端 statement_timeout，默认 30 秒；负数表示不设置。
	StatementTimeout time.Duration
	// SlowQueryThreshold 慢查询日志阈值，默认 200ms；负数表示关闭慢查询日志。
	SlowQueryThreshold time.Duration
	// ApplicationName 写入 application_name，便于在 pg_stat_activity 中识别服务，默认 "couponkill"。
	ApplicationName string
	// Logger 慢查询等日志输出，默认 slog.Default()。
	Logger *slog.Logger
}

// withDefaults 返回填充了默认值的副本。
func (o Options) withDefaults() Options {
	if o.MaxConns <= 0 {
		o.MaxConns = defaultMaxConns
	}
	if o.MinConns <= 0 {
		o.MinConns = defaultMinConns
	}
	if o.MinConns > o.MaxConns {
		o.MinConns = o.MaxConns
	}
	if o.MaxConnLifetime <= 0 {
		o.MaxConnLifetime = defaultMaxConnLifetime
	}
	if o.MaxConnIdleTime <= 0 {
		o.MaxConnIdleTime = defaultMaxConnIdleTime
	}
	if o.HealthCheckPeriod <= 0 {
		o.HealthCheckPeriod = defaultHealthCheckPeriod
	}
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = defaultConnectTimeout
	}
	if o.StatementTimeout == 0 {
		o.StatementTimeout = defaultStatementTimeout
	}
	if o.SlowQueryThreshold == 0 {
		o.SlowQueryThreshold = defaultSlowQuery
	}
	if o.ApplicationName == "" {
		o.ApplicationName = "couponkill"
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// buildConfig 解析 DSN 并套用 Options，返回可直接交给 pgxpool 的配置。
func buildConfig(dsn string, opts Options) (*pgxpool.Config, error) {
	opts = opts.withDefaults()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析 PostgreSQL DSN: %w", err)
	}
	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = opts.MinConns
	cfg.MaxConnLifetime = opts.MaxConnLifetime
	cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	cfg.HealthCheckPeriod = opts.HealthCheckPeriod
	cfg.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = opts.ApplicationName
	if opts.StatementTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", opts.StatementTimeout.Milliseconds())
	}
	if opts.SlowQueryThreshold > 0 {
		cfg.ConnConfig.Tracer = &slowQueryTracer{threshold: opts.SlowQueryThreshold, logger: opts.Logger}
	}
	return cfg, nil
}

// Open 按 dsn 与 opts 创建 pgx 连接池，并在 ConnectTimeout 内 Ping 一次以尽早暴露配置错误。
// 调用方负责在退出时 Close 返回的连接池。
func Open(ctx context.Context, dsn string, opts Options) (*pgxpool.Pool, error) {
	opts = opts.withDefaults()
	cfg, err := buildConfig(dsn, opts)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("创建连接池: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	return pool, nil
}

// Ready 返回适用于 /readyz 的检查函数：在 2 秒内对连接池做一次 Ping，失败即视为未就绪。
func Ready(pool *pgxpool.Pool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		pingCtx, cancel := context.WithTimeout(ctx, readyTimeout)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return fmt.Errorf("PostgreSQL 未就绪: %w", err)
		}
		return nil
	}
}

const readyTimeout = 2 * time.Second

type slowQueryKey struct{}

// queryStart 是 TraceQueryStart 放进 ctx 的上下文：开始时间与 SQL 文本。
type queryStart struct {
	at  time.Time
	sql string
}

// slowQueryTracer 记录执行时间超过阈值的查询。只记录 SQL 文本（截断）而不记录参数，避免日志泄露口令等敏感值。
type slowQueryTracer struct {
	threshold time.Duration
	logger    *slog.Logger
}

// TraceQueryStart 在 ctx 里记下开始时间。
func (t *slowQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, slowQueryKey{}, queryStart{at: time.Now(), sql: data.SQL})
}

// TraceQueryEnd 计算耗时，超过阈值则输出 Warn 日志。
func (t *slowQueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(slowQueryKey{}).(queryStart)
	if !ok {
		return
	}
	elapsed := time.Since(start.at)
	if elapsed < t.threshold {
		return
	}
	attrs := []any{"elapsed_ms", elapsed.Milliseconds(), "command_tag", data.CommandTag.String()}
	if data.Err != nil {
		attrs = append(attrs, "error", data.Err.Error())
	}
	t.logger.WarnContext(ctx, "慢查询", append(attrs, "sql", truncateSQL(start.sql))...)
}

// truncateSQL 压缩空白并截断过长的 SQL。
func truncateSQL(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) > slowQuerySQLMaxLen {
		return sql[:slowQuerySQLMaxLen] + "..."
	}
	return sql
}
