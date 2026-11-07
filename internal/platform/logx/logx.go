// Package logx 负责 slog 的初始化，以及把 logger / request id 随 context 传递。
//
// 所有输出为 JSON（便于 Loki/ELK 采集）；名字像密钥的属性在输出前自动打码，
// 因此即使有人误把令牌或口令传进日志，也不会落盘。
package logx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options 描述日志器配置。
type Options struct {
	// Service 服务名，作为每条日志的 service 字段；为空则不输出。
	Service string
	// Level 日志级别 debug|info|warn|error，空串视为 info。
	Level string
	// Writer 输出目的地，默认 os.Stdout。
	Writer io.Writer
}

// ParseLevel 解析日志级别（不区分大小写），空串为 info。
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("logx: unknown log level %q", s)
}

// sensitiveFragments 命中任一片段的属性键视为敏感。
var sensitiveFragments = []string{"password", "passwd", "secret", "token", "authorization", "sign_key", "signkey", "sig", "dsn"}

func isSensitive(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, f := range sensitiveFragments {
		if f == "sig" {
			if k == "sig" || strings.HasSuffix(k, "_sig") || strings.HasSuffix(k, "_signature") {
				return true
			}
			continue
		}
		if strings.Contains(k, f) {
			return true
		}
	}
	return false
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if isSensitive(a.Key) {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// New 构造 JSON logger，不修改全局状态。无效的级别回落为 info。
func New(o Options) *slog.Logger {
	lvl, err := ParseLevel(o.Level)
	w := o.Writer
	if w == nil {
		w = os.Stdout
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl, ReplaceAttr: redact})
	l := slog.New(h)
	if o.Service != "" {
		l = l.With("service", o.Service)
	}
	if err != nil {
		l.Warn("invalid log level, falling back to info", "level", o.Level)
	}
	return l
}

// Setup 构造 logger 并设为 slog 默认值，返回它。
func Setup(o Options) *slog.Logger {
	l := New(o)
	slog.SetDefault(l)
	return l
}

type ctxKey int

const (
	loggerKey ctxKey = iota
	requestIDKey
)

// NewContext 把 logger 放进 ctx。ctx 里已有 request id 时会自动附加到该 logger 上。
func NewContext(ctx context.Context, l *slog.Logger) context.Context {
	if id := RequestID(ctx); id != "" {
		l = l.With("request_id", id)
	}
	return context.WithValue(ctx, loggerKey, l)
}

// From 取出 ctx 里的 logger；没有则回落到 slog.Default()（并带上 ctx 里的 request id）。
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	l := slog.Default()
	if id := RequestID(ctx); id != "" {
		l = l.With("request_id", id)
	}
	return l
}

// With 给 ctx 里的 logger 追加字段并返回新 ctx。
func With(ctx context.Context, args ...any) context.Context {
	return context.WithValue(ctx, loggerKey, From(ctx).With(args...))
}

// WithRequestID 记录 request id，并让 From(ctx) 返回的日志自带 request_id 字段。
func WithRequestID(ctx context.Context, id string) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, id)
	return context.WithValue(ctx, loggerKey, From(ctx).With("request_id", id))
}

// RequestID 返回 ctx 里的 request id，没有则为空串。
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// NewRequestID 生成 32 位十六进制随机 id（128 bit）。
func NewRequestID() string {
	var b [16]byte
	// Go 1.24+ 的 crypto/rand.Read 永不返回错误（熵源损坏时由运行时直接终止进程）。
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
