package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/gateway"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// 测试用 JWT 参数，与 gateway 配置里的默认 issuer/audience 一致。
const (
	testIssuer   = "couponkill"
	testAudience = "couponkill-api"
)

// recorded 是假上游收到的一次请求。
type recorded struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// fakeUpstream 是一个记录请求的假上游（外部系统边界，用 httptest 替身）。
type fakeUpstream struct {
	*httptest.Server
	name string

	mu      sync.Mutex
	reqs    []recorded
	handler http.HandlerFunc
}

func newFakeUpstream(t *testing.T, name string) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{name: name}
	f.handler = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{"upstream":"`+name+`"}}`)
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recorded{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: body})
		h := f.handler
		f.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// Handle 替换上游的处理逻辑。
func (f *fakeUpstream) Handle(h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = h
}

// Requests 返回目前收到的全部请求。
func (f *fakeUpstream) Requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

// Count 返回目前收到的请求数。
func (f *fakeUpstream) Count() int { return len(f.Requests()) }

// Last 返回最近一次请求，没有则让测试失败。
func (f *fakeUpstream) Last(t *testing.T) recorded {
	t.Helper()
	rs := f.Requests()
	if len(rs) == 0 {
		t.Fatalf("upstream %s received no request", f.name)
	}
	return rs[len(rs)-1]
}

// env 是一套完整的被测网关：四个假上游、miniredis、假时钟与签发令牌的工具。
type env struct {
	t        *testing.T
	gw       *gateway.Gateway
	h        http.Handler
	cfg      gateway.Config
	rdb      *redis.Client
	mr       *miniredis.Miniredis
	clock    *testkit.Clock
	tokens   *authx.Tokens
	sessions *redisx.SessionStore

	user, coupon, order, connector *fakeUpstream
}

type envOpts struct {
	revoker authx.Revoker
}

// envOption 调整 newEnv。
type envOption func(*envOpts)

// withRevoker 用自定义会话存储替换 Redis 实现（用于模拟慢存储）。
func withRevoker(r authx.Revoker) envOption { return func(o *envOpts) { o.revoker = r } }

// newEnv 构造测试环境。extra 里的环境变量覆盖测试默认值（默认放宽限流，避免干扰与限流无关的用例）。
func newEnv(t *testing.T, extra map[string]string, opts ...envOption) *env {
	t.Helper()
	var o envOpts
	for _, fn := range opts {
		fn(&o)
	}
	e := &env{t: t}
	e.user = newFakeUpstream(t, "user")
	e.coupon = newFakeUpstream(t, "coupon")
	e.order = newFakeUpstream(t, "order")
	e.connector = newFakeUpstream(t, "connector")
	e.rdb, e.mr = testkit.Redis(t)
	e.clock = testkit.NewClock(time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
	e.sessions = redisx.NewSessionStore(e.rdb)

	m := map[string]string{
		"CK_USER_URL":                   e.user.URL,
		"CK_COUPON_URL":                 e.coupon.URL,
		"CK_ORDER_URL":                  e.order.URL,
		"CK_CONNECTOR_URL":              e.connector.URL,
		"CK_JWT_SECRET":                 testkit.JWTSecret,
		"CK_GATEWAY_SIGN_KEY":           testkit.GatewayKey,
		"CK_GATEWAY_IP_RPS":             "100000",
		"CK_GATEWAY_IP_BURST":           "100000",
		"CK_GATEWAY_USER_RPS":           "100000",
		"CK_GATEWAY_USER_BURST":         "100000",
		"CK_GATEWAY_SECKILL_RPS":        "100000",
		"CK_GATEWAY_SECKILL_BURST":      "100000",
		"CK_GATEWAY_CORS_ORIGINS":       "https://app.example.com",
		"CK_GATEWAY_UPSTREAM_TIMEOUT":   "2s",
		"CK_GATEWAY_SECKILL_TIMEOUT":    "1s",
		"CK_GATEWAY_REDIS_TIMEOUT":      "200ms",
		"CK_GATEWAY_BREAKER_FAILURES":   "3",
		"CK_GATEWAY_BREAKER_OPEN":       "30s",
		"CK_GATEWAY_MAX_BODY_BYTES":     "4096",
		"CK_GATEWAY_REVOCATION_ENABLED": "true",
	}
	for k, v := range extra {
		m[k] = v
	}
	cfg, err := gateway.LoadConfig(config.WithMap(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	e.cfg = cfg

	e.tokens = e.newTokens(testkit.JWTSecret, testIssuer, testAudience)
	var revoker authx.Revoker = e.sessions
	if o.revoker != nil {
		revoker = o.revoker
	}
	gw, err := gateway.New(gateway.Deps{
		Config: cfg, Revoker: revoker, Ping: redisx.PingFn(e.rdb), Now: e.clock.Now,
	})
	if err != nil {
		t.Fatalf("gateway.New: %v", err)
	}
	t.Cleanup(gw.Close)
	e.gw, e.h = gw, gw.Handler()
	return e
}

// newTokens 构造一个使用假时钟的令牌签发/校验器（可指定不同的密钥/iss/aud 来造伪令牌）。
func (e *env) newTokens(secret, iss, aud string) *authx.Tokens {
	e.t.Helper()
	tk, err := authx.NewTokens(authx.TokenConfig{Secret: secret, Issuer: iss, Audience: aud, TTL: time.Hour, Now: e.clock.Now})
	if err != nil {
		e.t.Fatalf("NewTokens: %v", err)
	}
	return tk
}

// login 为用户签发令牌并在 Redis 里登记会话，返回原始令牌与 jti。
func (e *env) login(userID int64, roles ...string) (raw, jti string) {
	e.t.Helper()
	return e.loginWith(e.tokens, userID, roles...)
}

// loginWith 用指定的签发器签发令牌，并登记会话（这样拒绝只可能来自 JWT 本身）。
func (e *env) loginWith(tk *authx.Tokens, userID int64, roles ...string) (raw, jti string) {
	e.t.Helper()
	tok, err := tk.Issue(authx.Identity{UserID: userID, Roles: roles})
	if err != nil {
		e.t.Fatalf("Issue: %v", err)
	}
	if err := e.sessions.Open(context.Background(), tok.JTI, time.Hour); err != nil {
		e.t.Fatalf("Open session: %v", err)
	}
	return tok.Raw, tok.JTI
}

// reqOpt 修改测试请求。
type reqOpt func(*http.Request)

func bearer(token string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func hdr(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func remote(addr string) reqOpt { return func(r *http.Request) { r.RemoteAddr = addr } }

func body(s string) reqOpt {
	return func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(s))
		r.ContentLength = int64(len(s))
	}
}

// result 是一次请求的响应及解析后的信封。
type result struct {
	*httptest.ResponseRecorder
	Code    int
	Message string
}

// do 对网关发起请求（不经过真实网络）。target 可以带原始转义，用来构造路径绕过变体。
func (e *env) do(method, target string, opts ...reqOpt) *result {
	e.t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(nil))
	req.RemoteAddr = "203.0.113.7:4000"
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	res := &result{ResponseRecorder: rec}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &env) == nil {
		res.Code, res.Message = env.Code, env.Message
	}
	return res
}

// serve 启动真实的 HTTP 服务器承载网关（流式、分块上传等需要真实连接的场景）。
func (e *env) serve() *httptest.Server {
	e.t.Helper()
	srv := httptest.NewServer(e.h)
	e.t.Cleanup(srv.Close)
	return srv
}

// verifyIdentity 用与下游服务相同的验签器检查上游收到的身份头，返回身份。
func verifyIdentity(t *testing.T, h http.Header, now time.Time) authx.Identity {
	t.Helper()
	signer, err := authx.NewGatewaySigner(testkit.GatewayKey, authx.WithSignerClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	id, err := signer.Verify(h)
	if err != nil {
		t.Fatalf("upstream received identity headers that do not verify: %v (headers %v)", err, h)
	}
	return id
}
