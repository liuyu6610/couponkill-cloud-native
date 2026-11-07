package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

const (
	defaultClientTimeout = 3 * time.Second
	defaultRetries       = 2
	maxResponseBytes     = 4 << 20
)

// ClientOptions 配置出站 HTTP 客户端。零值字段取默认值。
type ClientOptions struct {
	// BaseURL 形如 http://coupon:8080，调用时拼接 path。
	BaseURL string
	// Timeout 单次尝试的超时，默认 3s（可用 CallTimeout 逐次覆盖）。
	Timeout time.Duration
	// Retries 可重试调用的最大重试次数：0 取默认 2，负数表示不重试。
	Retries int
	// BackoffBase / BackoffMax 重试退避（指数 + 抖动），默认 50ms / 1s。
	BackoffBase time.Duration
	BackoffMax  time.Duration
	// InternalToken 非空则每个请求都带 X-Internal-Token。
	InternalToken string
	// Breaker 可选的熔断器：打开时直接失败，不发请求。
	Breaker *limiter.Breaker
	// HTTPClient 替换底层 http.Client（测试用）。
	HTTPClient *http.Client
}

// Client 是带超时、有界重试、request-id 传播与信封解码的 JSON HTTP 客户端。
//
// 错误约定：对方返回的业务错误（信封 code≠200 或非 2xx）还原为 *apperr.Error（码与状态取自对方，
// errors.Is(err, apperr.OutOfStock) 可用）；网络错误、超时、熔断、无法解析的响应统一表现为
// apperr.SystemBusy（503，原因在 cause 里）。调用方据 apperr.StatusOf(err) >= 500 判断“值得重试/稍后再试”。
type Client struct {
	opts ClientOptions
	hc   *http.Client
}

// NewClient 创建客户端。
func NewClient(opts ClientOptions) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultClientTimeout
	}
	switch {
	case opts.Retries == 0:
		opts.Retries = defaultRetries
	case opts.Retries < 0:
		opts.Retries = 0
	}
	if opts.BackoffBase <= 0 {
		opts.BackoffBase = 50 * time.Millisecond
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = time.Second
	}
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")
	hc := opts.HTTPClient
	if hc == nil {
		// 整体超时由每次尝试的 ctx 控制；这里只给传输层兜底，并限制空闲连接以复用长连接。
		hc = &http.Client{Transport: &http.Transport{
			MaxIdleConns: 100, MaxIdleConnsPerHost: 50, IdleConnTimeout: 90 * time.Second,
			ForceAttemptHTTP2: true,
		}}
	}
	return &Client{opts: opts, hc: hc}
}

type call struct {
	idempotent bool
	retries    *int
	timeout    time.Duration
	headers    http.Header
	signer     *authx.GatewaySigner
	identity   authx.Identity
}

// CallOption 调整单次调用。
type CallOption func(*call)

// Idempotent 声明本次调用幂等，因此（含 POST）网络错误与 502/503/504 时允许重试。
// 只有对方按幂等键去重（如 requestId）的接口才能使用。
func Idempotent() CallOption { return func(c *call) { c.idempotent = true } }

// Retries 覆盖本次调用的最大重试次数（仅对可重试的调用生效；0 表示不重试）。
func Retries(n int) CallOption { return func(c *call) { c.retries = &n } }

// CallTimeout 覆盖本次调用单次尝试的超时。
func CallTimeout(d time.Duration) CallOption { return func(c *call) { c.timeout = d } }

// Header 附加请求头。
func Header(key, value string) CallOption {
	return func(c *call) {
		if c.headers == nil {
			c.headers = http.Header{}
		}
		c.headers.Set(key, value)
	}
}

// AsIdentity 以 id 的身份调用（附网关签名头），用于经网关签名机制转发用户身份的场景。
func AsIdentity(signer *authx.GatewaySigner, id authx.Identity) CallOption {
	return func(c *call) { c.signer, c.identity = signer, id }
}

// Get 发起 GET。
func (c *Client) Get(ctx context.Context, path string, out any, opts ...CallOption) error {
	return c.Do(ctx, http.MethodGet, path, nil, out, opts...)
}

// Post 发起 POST，in 按 JSON 编码。
func (c *Client) Post(ctx context.Context, path string, in, out any, opts ...CallOption) error {
	return c.Do(ctx, http.MethodPost, path, in, out, opts...)
}

// Put 发起 PUT。
func (c *Client) Put(ctx context.Context, path string, in, out any, opts ...CallOption) error {
	return c.Do(ctx, http.MethodPut, path, in, out, opts...)
}

// Delete 发起 DELETE。
func (c *Client) Delete(ctx context.Context, path string, out any, opts ...CallOption) error {
	return c.Do(ctx, http.MethodDelete, path, nil, out, opts...)
}

func isIdempotentMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}

// outcome 是一次尝试的结果分类。
type outcome struct {
	err       error // 最终要返回的错误（成功为 nil）
	retryable bool  // 允许重试（仅网络错误与 502/503/504）
	upFailure bool  // 计入熔断器的失败（上游故障，而非业务拒绝）
}

// Do 发起一次 JSON 调用。in 为 nil 不带请求体；out 为 nil 忽略 data。
func (c *Client) Do(ctx context.Context, method, path string, in, out any, opts ...CallOption) error {
	var cc call
	for _, o := range opts {
		o(&cc)
	}
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return apperr.SysError.WithCause(fmt.Errorf("httpx: encode request: %w", err))
		}
		body = b
	}
	maxRetries := 0
	if isIdempotentMethod(method) || cc.idempotent {
		maxRetries = c.opts.Retries
		if cc.retries != nil {
			maxRetries = max(*cc.retries, 0)
		}
	}
	reqID := logx.RequestID(ctx)
	if reqID == "" {
		reqID = logx.NewRequestID()
	}

	for attempt := 0; ; attempt++ {
		var permit *limiter.Permit
		if br := c.opts.Breaker; br != nil {
			p, err := br.Allow()
			if err != nil {
				return apperr.SystemBusy.WithCause(fmt.Errorf("httpx: %s %s: %w", method, path, err))
			}
			permit = p
		}
		res := c.attempt(ctx, method, path, body, out, reqID, &cc)
		if permit != nil {
			permit.Done(!res.upFailure)
		}
		if res.err == nil {
			return nil
		}
		if !res.retryable || attempt >= maxRetries || ctx.Err() != nil {
			return res.err
		}
		delay := backoff(attempt, c.opts.BackoffBase, c.opts.BackoffMax)
		logx.From(ctx).Warn("outbound call failed, retrying",
			"method", method, "path", path, "attempt", attempt+1, "retry_in", delay, "err", res.err)
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return res.err
		case <-t.C:
		}
	}
}

func backoff(attempt int, base, maxDelay time.Duration) time.Duration {
	d := base
	for i := 0; i < attempt && d < maxDelay; i++ {
		d *= 2
	}
	d = min(d, maxDelay)
	return d + time.Duration(rand.Int64N(int64(d)/4+1)) // 最多 25% 抖动，避免重试风暴同步
}

func (c *Client) attempt(ctx context.Context, method, path string, body []byte, out any, reqID string, cc *call) outcome {
	timeout := c.opts.Timeout
	if cc.timeout > 0 {
		timeout = cc.timeout
	}
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(actx, method, c.opts.BaseURL+path, rd)
	if err != nil {
		return outcome{err: apperr.SysError.WithCause(fmt.Errorf("httpx: build request: %w", err))}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(HeaderRequestID, reqID)
	if c.opts.InternalToken != "" {
		req.Header.Set(HeaderInternalToken, c.opts.InternalToken)
	}
	if cc.signer != nil {
		if err := cc.signer.Apply(req.Header, cc.identity); err != nil {
			return outcome{err: apperr.SysError.WithCause(err)}
		}
	}
	for k, vs := range cc.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("httpx: %s %s: %w", method, path, err)
		// 调用方自己的 ctx 结束（取消/总时限）不是上游故障，也不该重试。
		if ctx.Err() != nil {
			return outcome{err: apperr.SystemBusy.WithCause(wrapped)}
		}
		return outcome{err: apperr.SystemBusy.WithCause(wrapped), retryable: true, upFailure: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		wrapped := fmt.Errorf("httpx: %s %s: read response: %w", method, path, err)
		return outcome{err: apperr.SystemBusy.WithCause(wrapped), retryable: ctx.Err() == nil, upFailure: ctx.Err() == nil}
	}
	return decode(method, path, resp.StatusCode, raw, out)
}

// decode 把响应翻译为 outcome：成功解出 data；失败还原为 apperr。
func decode(method, path string, status int, raw []byte, out any) outcome {
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	parseErr := json.Unmarshal(raw, &env)
	transient := status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
	serverFault := status >= http.StatusInternalServerError

	if status >= 200 && status < 300 {
		if parseErr != nil {
			return outcome{err: apperr.SystemBusy.WithCause(fmt.Errorf("httpx: %s %s: unparsable response: %w", method, path, parseErr)), upFailure: true}
		}
		if env.Code != apperr.CodeOK {
			return outcome{err: apperr.New(env.Code, status, env.Message)}
		}
		if out != nil && len(env.Data) > 0 && string(bytes.TrimSpace(env.Data)) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return outcome{err: apperr.SystemBusy.WithCause(fmt.Errorf("httpx: %s %s: decode data: %w", method, path, err)), upFailure: true}
			}
		}
		return outcome{}
	}

	cause := fmt.Errorf("httpx: %s %s -> HTTP %d", method, path, status)
	if parseErr != nil || env.Code == 0 || env.Code == apperr.CodeOK {
		code, msg := apperr.CodeInvalidRequest, "上游请求被拒绝"
		if serverFault {
			code, msg = apperr.CodeSysError, apperr.SysError.Message
		}
		return outcome{err: apperr.New(code, status, msg).WithCause(cause), retryable: transient, upFailure: serverFault}
	}
	return outcome{err: apperr.New(env.Code, status, env.Message).WithCause(cause), retryable: transient, upFailure: serverFault}
}
