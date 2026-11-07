package platforms

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Caller 默认值。
const (
	defaultRetries      = 2
	defaultBackoffBase  = 100 * time.Millisecond
	defaultBackoffMax   = time.Second
	defaultMaxBodyBytes = 1 << 20
	defaultConnTimeout  = 5 * time.Second
	defaultCallTimeout  = 10 * time.Second
	errSnippetBytes     = 200
)

// CallerConfig 配置 Caller。零值字段取默认值。
type CallerConfig struct {
	// Name 是平台名，写入日志与错误。
	Name string
	// Client 是底层 HTTP 客户端；默认 5s 建连、10s 整体超时（沿用 Java 版）。
	Client *http.Client
	// Retries 是首次尝试之后最多再试几次（仅传输错误、429、5xx）；0 取默认 2，负数表示不重试。
	Retries int
	// BackoffBase / BackoffMax 是指数退避的基数与上限。
	BackoffBase, BackoffMax time.Duration
	// Breaker 配置该平台的熔断器；IsFailure 由 Caller 接管，传入值被忽略。
	Breaker limiter.BreakerConfig
	// Sleep 是退避等待（测试注入）；必须在 ctx 取消时返回 ctx 的错误。
	Sleep func(ctx context.Context, d time.Duration) error
	// MaxBodyBytes 是响应体上限，默认 1 MiB；超限视为 ErrBadResponse。
	MaxBodyBytes int64
}

// Caller 是各平台适配器共用的出站 HTTP 封装：统一超时、对幂等读有界重试、
// 每平台一个熔断器。它只发送“读”请求（查询商品/库存），因此重试是安全的。并发安全。
type Caller struct {
	name    string
	client  *http.Client
	retries int
	base    time.Duration
	maxWait time.Duration
	sleep   func(context.Context, time.Duration) error
	maxBody int64
	breaker *limiter.Breaker
}

// NewCaller 创建 Caller。
func NewCaller(cfg CallerConfig) *Caller {
	if cfg.Client == nil {
		cfg.Client = &http.Client{
			Timeout:   defaultCallTimeout,
			Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: defaultConnTimeout}).DialContext, MaxIdleConnsPerHost: 8},
		}
	}
	switch {
	case cfg.Retries == 0:
		cfg.Retries = defaultRetries
	case cfg.Retries < 0:
		cfg.Retries = 0
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = defaultBackoffBase
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = defaultBackoffMax
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	cfg.Breaker.IsFailure = func(err error) bool {
		var up *UpstreamError
		return errors.As(err, &up) && up.Retryable()
	}
	userHook := cfg.Breaker.OnStateChange
	name := cfg.Name
	cfg.Breaker.OnStateChange = func(from, to limiter.State) {
		logx.From(context.Background()).Warn("platform circuit breaker state changed", "platform", name, "from", from.String(), "to", to.String())
		if userHook != nil {
			userHook(from, to)
		}
	}
	return &Caller{
		name: cfg.Name, client: cfg.Client, retries: cfg.Retries,
		base: cfg.BackoffBase, maxWait: cfg.BackoffMax, sleep: cfg.Sleep, maxBody: cfg.MaxBodyBytes,
		breaker: limiter.NewBreaker(cfg.Breaker),
	}
}

// BreakerState 返回熔断器当前状态，供适配器据此报告 UP/DOWN。
func (c *Caller) BreakerState() limiter.State { return c.breaker.State() }

// Do 执行一次逻辑调用并返回 2xx 响应体。build 在每次尝试时重新调用（重新签名、刷新时间戳），
// 且必须用传入的 ctx 构造请求。熔断器打开时直接返回 ErrCircuitOpen，不发请求。
// 整个重试循环算熔断器的一次调用：重试耗尽才记一次失败。
func (c *Caller) Do(ctx context.Context, build func(ctx context.Context) (*http.Request, error)) ([]byte, error) {
	var body []byte
	err := c.breaker.Execute(func() error {
		b, err := c.withRetries(ctx, build)
		body = b
		return err
	})
	if errors.Is(err, limiter.ErrOpen) {
		return nil, fmt.Errorf("%s: %w", c.name, ErrCircuitOpen)
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

// withRetries 是重试循环：只重试可重试的 UpstreamError，调用方 ctx 结束立即返回。
func (c *Caller) withRetries(ctx context.Context, build func(ctx context.Context) (*http.Request, error)) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, backoff(attempt, c.base, c.maxWait)); err != nil {
				return nil, fmt.Errorf("%s: retry wait aborted: %w", c.name, err)
			}
		}
		body, err := c.once(ctx, build)
		if err == nil {
			return body, nil
		}
		lastErr = err
		var up *UpstreamError
		if !errors.As(err, &up) || !up.Retryable() || ctx.Err() != nil {
			return nil, err
		}
		logx.From(ctx).Debug("platform call failed, will retry", "platform", c.name, "attempt", attempt+1, "err", err)
	}
	return nil, lastErr
}

// once 发出一次请求并分类结果。
func (c *Caller) once(ctx context.Context, build func(ctx context.Context) (*http.Request, error)) ([]byte, error) {
	req, err := build(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", c.name, err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s: %w", c.name, ctxErr)
		}
		return nil, &UpstreamError{Platform: c.name, Err: stripURL(err)}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s: %w", c.name, ctxErr)
		}
		return nil, &UpstreamError{Platform: c.name, Status: resp.StatusCode, Err: fmt.Errorf("read body: %w", err)}
	}
	if int64(len(raw)) > c.maxBody {
		return nil, fmt.Errorf("%s: response larger than %d bytes: %w", c.name, c.maxBody, ErrBadResponse)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &UpstreamError{Platform: c.name, Status: resp.StatusCode, Err: errors.New(snippet(raw))}
	}
	return raw, nil
}

// stripURL 去掉 *url.Error 里携带的请求 URL，避免带签名/凭证的查询串进入错误文案与日志。
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// backoff 返回第 attempt（从 1 起）次重试前的等待：base*2^(attempt-1)，封顶 maxWait。
func backoff(attempt int, base, maxWait time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt && d < maxWait; i++ {
		d *= 2
	}
	return min(d, maxWait)
}

// sleepCtx 等待 d，ctx 先结束则返回其错误。
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// snippet 截取响应体开头用于错误描述（已限长，避免日志与错误文案过长）。
func snippet(raw []byte) string {
	if len(raw) > errSnippetBytes {
		raw = raw[:errSnippetBytes]
	}
	return string(raw)
}
