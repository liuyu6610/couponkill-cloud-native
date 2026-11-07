package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Deps 是构造网关所需的依赖。外部系统（Redis 会话存储）以接口注入，测试可替换。
type Deps struct {
	// Config 已校验的配置（LoadConfig）。
	Config Config
	// Logger 默认 slog.Default()。
	Logger *slog.Logger
	// Revoker 会话存储；Config.RevocationEnabled 为 true 时必填。
	Revoker authx.Revoker
	// Ping 会话存储的就绪探测（redisx.PingFn）；非 nil 且启用吊销检查时挂到 /readyz。
	Ping func(context.Context) error
	// Now 注入时钟（限流、熔断、身份签名），默认 time.Now。
	Now func() time.Time
}

// Gateway 是装配好的网关。零值不可用，请用 New。
type Gateway struct {
	cfg     Config
	log     *slog.Logger
	engine  *httpx.Engine
	tokens  *authx.Tokens
	signer  *authx.GatewaySigner
	revoker authx.Revoker
	trusted []netip.Prefix
	cors    corsPolicy
	metrics *gwMetrics

	upstreams    map[string]*upstream
	ipLimit      *limiter.Buckets
	userLimit    *limiter.Buckets
	seckillLimit *limiter.Buckets
}

// New 校验依赖并装配路由、中间件与上游。
func New(d Deps) (*Gateway, error) {
	cfg := d.Config
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if cfg.RevocationEnabled && d.Revoker == nil {
		return nil, errors.New("gateway: session revocation is enabled but no session store was provided")
	}

	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: cfg.JWTSecret.Reveal(), Issuer: cfg.JWTIssuer, Audience: cfg.JWTAudience, TTL: cfg.JWTTTL, Now: d.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("gateway: init jwt verifier: %w", err)
	}
	signer, err := authx.NewGatewaySigner(cfg.GatewaySignKey.Reveal(), authx.WithSignerClock(d.Now))
	if err != nil {
		return nil, fmt.Errorf("gateway: init identity signer: %w", err)
	}
	trusted := make([]netip.Prefix, 0, len(cfg.TrustedProxies))
	for _, p := range cfg.TrustedProxies {
		prefix, err := parseProxyPrefix(p)
		if err != nil {
			return nil, fmt.Errorf("gateway: trusted proxies: %w", err)
		}
		trusted = append(trusted, prefix)
	}

	g := &Gateway{
		cfg: cfg, log: d.Logger, tokens: tokens, signer: signer, revoker: d.Revoker,
		trusted: trusted, cors: newCORSPolicy(cfg.CORSOrigins),
		upstreams: make(map[string]*upstream, 4),
		ipLimit:   limiter.NewBuckets(limiter.BucketConfig{Rate: cfg.IPRate, Burst: cfg.IPBurst, Now: d.Now}),
		userLimit: limiter.NewBuckets(limiter.BucketConfig{Rate: cfg.UserRate, Burst: cfg.UserBurst, Now: d.Now}),
		seckillLimit: limiter.NewBuckets(limiter.BucketConfig{
			Rate: cfg.SeckillRate, Burst: cfg.SeckillBurst, Now: d.Now,
		}),
	}

	g.engine = httpx.NewEngine(httpx.Options{
		Service: "gateway", Logger: d.Logger, MaxBodyBytes: cfg.MaxBodyBytes, TrustedProxies: cfg.TrustedProxies,
	})
	names := []string{upstreamUser, upstreamCoupon, upstreamOrder, upstreamConnector}
	g.metrics = newGWMetrics(g.engine.Registry, names)

	timeouts := map[string]time.Duration{
		upstreamUser: cfg.UpstreamTimeout, upstreamCoupon: cfg.UpstreamTimeout,
		upstreamOrder: cfg.UpstreamTimeout, upstreamConnector: cfg.ConnectorTimeout,
	}
	for name, raw := range cfg.upstreamURLs() {
		br := limiter.NewBreaker(limiter.BreakerConfig{
			FailureThreshold: cfg.BreakerFailures, OpenTimeout: cfg.BreakerOpen, Now: d.Now,
			OnStateChange: g.breakerChanged(name),
		})
		up, err := newUpstream(name, raw, timeouts[name], br, g)
		if err != nil {
			return nil, err
		}
		g.upstreams[name] = up
	}

	if cfg.RevocationEnabled && d.Ping != nil {
		g.engine.Health.Add("redis", d.Ping)
	}
	g.mount()
	return g, nil
}

// mount 注册中间件链。/healthz、/readyz、/metrics 由 httpx 在中间件之前注册，因此不受鉴权与限流影响；
// 其余所有请求都落入 NoRoute，按下面的顺序处理，最后一个处理器是反向代理。
func (g *Gateway) mount() {
	g.engine.Use(
		g.securityHeaders,
		g.guardPath,
		g.handleCORS,
		g.limitIP,
		g.resolveRoute,
		g.limitBody,
		g.authenticate,
		g.limitUser,
		g.limitSeckill,
	)
	g.engine.NoRoute(g.serveProxy)
}

// Handler 返回对外的 http.Handler。它在做任何事之前先剥离入站请求里的所有身份头，
// 因此无论请求走哪条路径（包括免登录路径与探针），客户端都无法伪造身份。
func (g *Gateway) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authx.StripIdentityHeaders(r.Header)
		g.engine.ServeHTTP(w, r)
	})
}

// Engine 返回底层引擎，用于挂接就绪检查（Health）与读取指标注册表。
func (g *Gateway) Engine() *httpx.Engine { return g.engine }

// Close 关闭到上游的空闲连接。
func (g *Gateway) Close() {
	for _, up := range g.upstreams {
		up.transport.CloseIdleConnections()
	}
}

// breakerChanged 返回某个上游熔断器的状态变化回调：打日志并更新指标。
func (g *Gateway) breakerChanged(name string) func(from, to limiter.State) {
	return func(from, to limiter.State) {
		g.metrics.breaker.WithLabelValues(name).Set(breakerGauge(to))
		g.log.Warn("upstream circuit breaker changed", "upstream", name, "from", from.String(), "to", to.String())
	}
}

// isTrustedPeer 报告直连对端 IP 是否属于受信代理。
func (g *Gateway) isTrustedPeer(ip string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	for _, p := range g.trusted {
		if p.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}

// reqState 是一个请求在中间件链上累积的状态。
type reqState struct {
	route    route
	public   bool
	identity *authx.Identity
}

const stateKey = "gateway.state"

func stateOf(c *gin.Context) *reqState {
	v, _ := c.Get(stateKey)
	st, _ := v.(*reqState)
	return st
}

// serveProxy 是 NoRoute 链上的最后一个处理器：把请求转发给路由命中的上游。
func (g *Gateway) serveProxy(c *gin.Context) {
	st := stateOf(c)
	if st == nil { // resolveRoute 总会先写入状态；到这里说明装配出错，宁可 500 也不要乱转发
		g.fail(c, "", errors.New("gateway: request reached the proxy without a resolved route"))
		return
	}
	g.forward(c, st)
}

// fail 写出错误信封并按原因计数（reason 为空则不计入拒绝指标）。
func (g *Gateway) fail(c *gin.Context, reason string, err error) {
	if reason != "" {
		g.metrics.rejected.WithLabelValues(reason).Inc()
		logx.From(c.Request.Context()).Debug("gateway rejected request", "reason", reason, "path", c.Request.URL.Path)
	}
	httpx.Fail(c, err)
}
