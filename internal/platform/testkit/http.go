package testkit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
)

// 仅供测试使用的固定密钥（均 ≥32 字节，满足各构造函数的长度要求）。
const (
	GatewayKey    = "testkit-gateway-sign-key-0123456789abcdef"
	InternalToken = "testkit-internal-token-0123456789abcdef"
	JWTSecret     = "testkit-jwt-secret-0123456789abcdef-xyz"
)

// 与 httpx 约定一致的头名称（testkit 不依赖 httpx，所以在此重复声明）。
const (
	headerInternalToken = "X-Internal-Token"
	headerRequestID     = "X-Request-Id"
)

// AsUser 给请求附上有效的网关签名身份头（密钥为 GatewayKey，时间戳为当前时间），
// 让下游服务的 handler 测试可以直接以某个用户的身份发起请求。返回同一个 req 便于链式使用。
func AsUser(req *http.Request, id int64, roles ...string) *http.Request {
	authx.StripIdentityHeaders(req.Header)
	ts := time.Now().Unix()
	req.Header.Set(authx.HeaderUserID, strconv.FormatInt(id, 10))
	req.Header.Set(authx.HeaderUserRoles, strings.Join(roles, ","))
	req.Header.Set(authx.HeaderGatewayTS, strconv.FormatInt(ts, 10))
	req.Header.Set(authx.HeaderGatewaySig, authx.SignGateway([]byte(GatewayKey), id, roles, ts))
	return req
}

// WithInternalToken 给请求附上 InternalToken，用于测试 /internal/v1/** 端点。
func WithInternalToken(req *http.Request) *http.Request {
	req.Header.Set(headerInternalToken, InternalToken)
	return req
}

// ReqOption 修改 DoJSON 构造出的请求。
type ReqOption func(*http.Request)

// User 以某个用户身份（网关签名头）发起请求。
func User(id int64, roles ...string) ReqOption {
	return func(r *http.Request) { AsUser(r, id, roles...) }
}

// Internal 附带内部调用令牌。
func Internal() ReqOption { return func(r *http.Request) { WithInternalToken(r) } }

// Header 设置任意请求头。
func Header(key, value string) ReqOption { return func(r *http.Request) { r.Header.Set(key, value) } }

// RemoteAddr 设置请求来源地址（测试按 IP 限流等场景）。
func RemoteAddr(addr string) ReqOption { return func(r *http.Request) { r.RemoteAddr = addr } }

// Response 是 DoJSON 的结果，已按信封格式解析。
type Response struct {
	t      testing.TB
	Status int
	Header http.Header
	// Body 是原始响应体。
	Body []byte
	// Code / Message 来自信封；响应体不是信封（如 /metrics）时 Code 为 0。
	Code    int
	Message string
	// RequestID 来自信封的 requestId 字段（仅失败响应带）。
	RequestID string
	data      json.RawMessage
}

// Decode 把信封的 data 解析进 v；失败则让测试立即失败。
func (r *Response) Decode(v any) {
	r.t.Helper()
	if len(r.data) == 0 {
		r.t.Fatalf("response has no data to decode (status %d, body %s)", r.Status, r.Body)
	}
	if err := json.Unmarshal(r.data, v); err != nil {
		r.t.Fatalf("decode data %s: %v", r.data, err)
	}
}

// DataIsNull 报告信封的 data 是否为 null/缺失。
func (r *Response) DataIsNull() bool {
	return len(r.data) == 0 || string(bytes.TrimSpace(r.data)) == "null"
}

// DoJSON 对 handler 发起一次请求并解析信封。body 为 nil 不带请求体；string/[]byte 原样发送
// （用于构造非法 JSON）；其他值按 JSON 编码。
func DoJSON(t testing.TB, h http.Handler, method, path string, body any, opts ...ReqOption) *Response {
	t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := &Response{t: t, Status: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	var env struct {
		Code      int             `json:"code"`
		Message   string          `json:"message"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"requestId"`
	}
	if json.Unmarshal(res.Body, &env) == nil {
		res.Code, res.Message, res.RequestID, res.data = env.Code, env.Message, env.RequestID, env.Data
	}
	return res
}

// Signer 返回使用 GatewayKey 的网关签名器，用来构造被测的 httpx.RequireGatewayIdentity。
func Signer(t testing.TB) *authx.GatewaySigner {
	t.Helper()
	gs, err := authx.NewGatewaySigner(GatewayKey)
	if err != nil {
		t.Fatalf("testkit: gateway signer: %v", err)
	}
	return gs
}

// Clock 是可手动推进的假时钟，并发安全。把 clk.Now 传给任何接受 func() time.Time 的注入点。
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock 创建从 start 起步的假时钟。
func NewClock(start time.Time) *Clock { return &Clock{t: start} }

// Now 返回当前假时间。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance 把时间向前推进 d（可为负，用来模拟回拨）。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Set 直接设置当前时间。
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}
