package internalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// maxResponseBytes 限制读取的响应体大小，防止异常下游拖垮调用方内存。
const maxResponseBytes = 1 << 20

// ctxKey 是 context 里请求 ID 的键类型。
type ctxKey struct{}

// WithRequestID 返回携带请求 ID 的 context；客户端会把它放进 X-Request-Id 头。
// 如果调用方已有自己的请求 ID 来源（例如 logx），改用 WithRequestIDSource。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFromContext 返回 WithRequestID 放入的请求 ID，没有则为空串。
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// RetryPolicy 是幂等调用的重试策略：指数退避加抖动，次数有上限。
type RetryPolicy struct {
	// MaxAttempts 是含首次调用在内的最大尝试次数；小于 1 视为 1（不重试）。
	MaxAttempts int
	// BaseDelay 是第一次重试前的基准等待，之后每次翻倍。
	BaseDelay time.Duration
	// MaxDelay 是单次等待的上限。
	MaxDelay time.Duration
}

// DefaultRetryPolicy 是默认策略：最多 3 次尝试，等待约 50ms、100ms（各带抖动），上限 500ms。
var DefaultRetryPolicy = RetryPolicy{MaxAttempts: 3, BaseDelay: 50 * time.Millisecond, MaxDelay: 500 * time.Millisecond}

// backoff 返回第 attempt 次失败（从 1 开始）之后的等待：
// 基准 = min(MaxDelay, BaseDelay<<(attempt-1))，实际取 [基准/2, 基准] 的随机值（equal jitter）。
func (p RetryPolicy) backoff(attempt int) time.Duration {
	if p.BaseDelay <= 0 {
		return 0
	}
	d := p.BaseDelay
	for i := 1; i < attempt && d < p.MaxDelay; i++ {
		d *= 2
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// Option 调整客户端行为。
type Option func(*transport)

// WithRetryPolicy 覆盖幂等调用的重试策略。
func WithRetryPolicy(p RetryPolicy) Option {
	return func(t *transport) { t.retry = p }
}

// WithRequestIDSource 指定从 context 取请求 ID 的函数，默认读取 WithRequestID 放入的值。
// 服务里通常接上 logx 的请求 ID 读取函数，使日志与下游调用共用同一个 ID。
func WithRequestIDSource(f func(context.Context) string) Option {
	return func(t *transport) {
		if f != nil {
			t.requestID = f
		}
	}
}

// WithLogger 指定重试日志使用的 logger，默认 slog.Default()。
func WithLogger(l *slog.Logger) Option {
	return func(t *transport) {
		if l != nil {
			t.log = l
		}
	}
}

// DefaultHTTPClient 返回带有合理超时与连接池的 http.Client：单次尝试总超时 3s，
// 连接/TLS 握手 1s，每主机保持 64 个空闲连接（秒杀消费者对 coupon 是高频调用）。
func DefaultHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = time.Second
	tr.MaxIdleConns = 256
	tr.MaxIdleConnsPerHost = 64
	tr.IdleConnTimeout = 90 * time.Second
	return &http.Client{Transport: tr, Timeout: 3 * time.Second}
}

// transport 是三个类型化客户端共用的底层：URL、令牌、头、信封解码、错误映射与重试。
type transport struct {
	service     string
	base        string
	token       string
	hc          *http.Client
	retry       RetryPolicy
	requestID   func(context.Context) string
	log         *slog.Logger
	unavailable *apperr.Error
}

// newTransport 校验基础地址与令牌并装配 transport。
// unavailable 是无信封 5xx 响应对应的业务错误（coupon 为 CouponServiceUnavailable，其余 SystemBusy）。
func newTransport(service, baseURL, token string, hc *http.Client, unavailable *apperr.Error, opts []Option) (*transport, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("internalapi: invalid %s base URL %q", service, baseURL)
	}
	if token == "" {
		return nil, fmt.Errorf("internalapi: empty internal token for %s client", service)
	}
	if hc == nil {
		hc = DefaultHTTPClient()
	}
	t := &transport{
		service:     service,
		base:        strings.TrimRight(u.String(), "/"),
		token:       token,
		hc:          hc,
		retry:       DefaultRetryPolicy,
		requestID:   RequestIDFromContext,
		log:         slog.Default(),
		unavailable: unavailable,
	}
	for _, o := range opts {
		o(t)
	}
	return t, nil
}

// envelope 是平台统一信封 {code,message,data}。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call 发起一次逻辑调用。idempotent 为 true 时，transient 失败（网络错误、超时、5xx）按策略重试；
// 4xx 与业务错误永不重试；ctx 取消立即停止并返回包装了 ctx.Err() 的错误。
// op 是日志用的操作名；body 为 nil 表示无请求体；out 为 nil 表示不关心 data。
func (t *transport) call(ctx context.Context, op, method, path string, body, out any, idempotent bool) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return apperr.SysError.WithCause(fmt.Errorf("internalapi %s: marshal request: %w", op, err))
		}
	}
	attempts := 1
	if idempotent && t.retry.MaxAttempts > 1 {
		attempts = t.retry.MaxAttempts
	}
	for attempt := 1; ; attempt++ {
		err := t.attempt(ctx, op, method, path, payload, out)
		if err == nil {
			return nil
		}
		if !IsTransient(err) || attempt >= attempts {
			return err
		}
		delay := t.retry.backoff(attempt)
		t.log.Warn("internal call failed, retrying",
			slog.String("service", t.service), slog.String("op", op),
			slog.Int("attempt", attempt), slog.Int("maxAttempts", attempts),
			slog.Duration("backoff", delay), slog.Any("error", err))
		if err := sleep(ctx, delay); err != nil {
			return fmt.Errorf("internalapi %s: retry aborted: %w", op, err)
		}
	}
}

// sleep 等待 d 或 ctx 结束，ctx 结束时返回 ctx.Err()。
func sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// attempt 执行一次 HTTP 往返并把结果归类为 nil / transient / 业务错误 / ctx 错误。
func (t *transport) attempt(ctx context.Context, op, method, path string, payload []byte, out any) error {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, reader)
	if err != nil {
		return apperr.SysError.WithCause(fmt.Errorf("internalapi %s: build request: %w", op, err))
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(HeaderInternalToken, t.token)
	if id := t.requestID(ctx); id != "" {
		req.Header.Set(HeaderRequestID, id)
	}

	resp, err := t.hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("internalapi %s: %w", op, ctxErr)
		}
		return markTransient(apperr.SystemBusy.WithCause(fmt.Errorf("internalapi %s: %w", op, err)))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("internalapi %s: %w", op, ctxErr)
		}
		return markTransient(apperr.SystemBusy.WithCause(fmt.Errorf("internalapi %s: read response: %w", op, err)))
	}
	return t.decode(op, resp.StatusCode, raw, out)
}

// decode 把一次响应解成成功结果或错误。
func (t *transport) decode(op string, status int, raw []byte, out any) error {
	var env envelope
	envErr := json.Unmarshal(raw, &env)
	validEnvelope := envErr == nil && env.Code != 0

	if status < 200 || status >= 300 {
		cause := fmt.Errorf("internalapi %s: HTTP %d", op, status)
		if !validEnvelope || env.Code == apperr.CodeOK {
			return remoteError(status, 0, "", t.unavailable, cause)
		}
		return remoteError(status, env.Code, env.Message, t.unavailable, cause)
	}
	if !validEnvelope {
		return apperr.SysError.WithCause(fmt.Errorf("internalapi %s: malformed envelope (HTTP %d): %v", op, status, envErr))
	}
	if env.Code != apperr.CodeOK {
		return remoteError(status, env.Code, env.Message, t.unavailable,
			fmt.Errorf("internalapi %s: envelope code %d", op, env.Code))
	}
	if out == nil {
		return nil
	}
	if len(env.Data) == 0 || bytes.Equal(bytes.TrimSpace(env.Data), []byte("null")) {
		return apperr.SysError.WithCause(fmt.Errorf("internalapi %s: success envelope without data", op))
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return apperr.SysError.WithCause(fmt.Errorf("internalapi %s: decode data: %w", op, err))
	}
	return nil
}
