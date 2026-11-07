package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// errUpstreamTimeout 是上游在时限内没有返回响应头时记录的取消原因。
var errUpstreamTimeout = errors.New("gateway: upstream response header timeout")

// 网关自己生成的、不暴露上游细节的错误。HTTP 状态码区分「连不上/出错」与「超时」。
var (
	errNotFound     = apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "接口不存在")
	errBadPath      = apperr.InvalidRequest.WithMessage("请求路径不合法")
	errBodyTooLarge = apperr.New(apperr.CodeInvalidRequest, http.StatusRequestEntityTooLarge, "请求体过大")
)

// upstream 是一个上游服务：基础 URL、独立的连接池、熔断器和对应的反向代理。
type upstream struct {
	name      string
	timeout   time.Duration
	breaker   *limiter.Breaker
	proxy     *httputil.ReverseProxy
	transport *http.Transport
	// unavailable 是熔断打开/上游出错时对外使用的业务错误（coupon 上游有专用码 10013）。
	unavailable *apperr.Error
}

// callInfo 是一次代理调用的上下文，经请求 ctx 传给 ReverseProxy 的回调。
// 它只在处理该请求的 goroutine 里读写（定时器回调只调用 cancel）。
type callInfo struct {
	gc       *gin.Context
	up       *upstream
	clientIP string
	proto    string
	permit   *limiter.Permit
	settled  bool
	timer    *time.Timer
}

type callKey struct{}

func callFrom(ctx context.Context) *callInfo {
	c, _ := ctx.Value(callKey{}).(*callInfo)
	return c
}

// settle 向熔断器上报一次调用结果，只生效一次。
func (c *callInfo) settle(success bool) {
	if c.settled {
		return
	}
	c.settled = true
	c.permit.Done(success)
}

// newUpstream 构造上游。每个上游有独立的 Transport，避免一个上游的慢连接占满其他上游的连接池。
func newUpstream(name, rawURL string, timeout time.Duration, br *limiter.Breaker, g *Gateway) (*upstream, error) {
	target, err := parseUpstreamURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("gateway: upstream %s: %w", name, err)
	}
	tr := &http.Transport{
		Proxy:                 nil, // 内部调用绝不走环境里的 HTTP_PROXY
		DialContext:           (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true, // 原样转发客户端协商的编码，不替客户端解压
	}
	u := &upstream{name: name, timeout: timeout, breaker: br, transport: tr, unavailable: apperr.SystemBusy}
	if name == upstreamCoupon {
		u.unavailable = apperr.CouponServiceUnavailable
	}
	u.proxy = &httputil.ReverseProxy{
		Transport: tr,
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteRequest(pr, target)
		},
		ModifyResponse: g.modifyResponse,
		ErrorHandler:   g.proxyError,
		ErrorLog:       slog.NewLogLogger(g.log.Handler(), slog.LevelWarn),
	}
	return u, nil
}

// rewriteRequest 把入站请求改写成发往上游的请求：保留方法、路径、查询与请求体（流式），
// 补上 X-Forwarded-* 与 X-Request-Id。身份头已在入站时被剥离，已认证请求的签名头在转发前写好。
func rewriteRequest(pr *httputil.ProxyRequest, target *url.URL) {
	pr.SetURL(target)
	pr.SetXForwarded()
	if call := callFrom(pr.In.Context()); call != nil {
		pr.Out.Header.Set("X-Forwarded-For", call.clientIP)
		pr.Out.Header.Set("X-Forwarded-Proto", call.proto)
	}
	pr.Out.Header.Set(httpx.HeaderRequestID, logx.RequestID(pr.In.Context()))
}

// gatewayOwnedResponseHeaders 是网关在响应上统一设置的头；上游若也带了，丢弃上游的以免重复。
var gatewayOwnedResponseHeaders = []string{
	httpx.HeaderRequestID,
	"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy",
	"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers", "Access-Control-Expose-Headers", "Access-Control-Max-Age",
}

// modifyResponse 在收到上游响应头时调用：停止响应头计时器、向熔断器上报结果、整理响应头。
// 只有 5xx 算上游失败；4xx 是业务拒绝，不影响熔断。
func (g *Gateway) modifyResponse(res *http.Response) error {
	call := callFrom(res.Request.Context())
	if call == nil {
		return errors.New("gateway: proxy response without call context")
	}
	call.timer.Stop()
	call.settle(res.StatusCode < http.StatusInternalServerError)
	for _, h := range gatewayOwnedResponseHeaders {
		res.Header.Del(h)
	}
	if res.Header.Get("Cache-Control") != "" {
		call.gc.Writer.Header().Del("Cache-Control") // 上游明确声明了缓存策略就听上游的
	}
	return nil
}

// proxyError 处理拿不到上游响应头的情况，把原因映射成信封，绝不把上游的错误文本带给客户端：
//
//   - 响应头超时 → 504；
//   - 客户端自己断开 → 499（无响应体），不算上游失败；
//   - 请求体超过上限 → 413，不算上游失败；
//   - 其余（连接被拒、重置……）→ 502。
func (g *Gateway) proxyError(_ http.ResponseWriter, req *http.Request, err error) {
	call := callFrom(req.Context())
	if call == nil {
		return
	}
	ctx := req.Context()
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(context.Cause(ctx), errUpstreamTimeout):
		call.settle(false)
		g.fail(call.gc, "", call.up.upstreamErr(http.StatusGatewayTimeout, "上游服务响应超时").WithCause(err))
	case errors.As(err, &tooLarge):
		call.settle(true)
		g.fail(call.gc, reasonBodyTooLarge, errBodyTooLarge.WithCause(err))
	case errors.Is(err, context.Canceled):
		call.settle(true)
		logx.From(ctx).Debug("client canceled request", "upstream", call.up.name)
		call.gc.AbortWithStatus(statusClientClosed)
	default:
		call.settle(false)
		g.fail(call.gc, "", call.up.upstreamErr(http.StatusBadGateway, "上游服务不可用").WithCause(err))
	}
}

// statusClientClosed 是客户端在响应前断开时记录的状态码（沿用 nginx 的 499 约定）。
const statusClientClosed = 499

// upstreamErr 构造网关自己的 502/504 信封：业务码沿用该上游的「不可用」码，HTTP 状态码按场景区分。
func (u *upstream) upstreamErr(status int, message string) *apperr.Error {
	return apperr.New(u.unavailable.Code, status, message)
}

// forward 把请求交给上游：先过熔断器，再带着响应头计时器调用 ReverseProxy。
func (g *Gateway) forward(c *gin.Context, st *reqState) {
	up := g.upstreams[st.route.upstream]
	permit, err := up.breaker.Allow()
	if err != nil {
		g.fail(c, reasonBreakerOpen, up.unavailable.WithMessage("服务暂时不可用，请稍后再试").WithCause(err))
		return
	}

	timeout := up.timeout
	if isSeckillEntry(c.Request.Method, c.Request.URL.Path) {
		timeout = g.cfg.SeckillTimeout
	}
	ctx, cancel := context.WithCancelCause(c.Request.Context())
	call := &callInfo{gc: c, up: up, clientIP: c.ClientIP(), proto: g.forwardedProto(c), permit: permit}
	call.timer = time.AfterFunc(timeout, func() { cancel(errUpstreamTimeout) })
	start := time.Now()
	defer func() {
		call.timer.Stop()
		cancel(nil)
		call.settle(false) // 正常路径在响应头/错误回调里都已上报；走到这里说明转发被异常中止，按失败计
		g.observe(c, st, up, time.Since(start))
	}()

	c.Request = c.Request.WithContext(context.WithValue(ctx, callKey{}, call))
	up.proxy.ServeHTTP(c.Writer, c.Request)
}

// observe 记录一次转发的指标与访问日志（含 userId、上游、耗时、状态）。
func (g *Gateway) observe(c *gin.Context, st *reqState, up *upstream, d time.Duration) {
	status := c.Writer.Status()
	g.metrics.proxied.WithLabelValues(up.name, fmt.Sprint(status)).Inc()
	g.metrics.latency.WithLabelValues(up.name).Observe(d.Seconds())
	attrs := []any{
		"upstream", up.name, "status", status,
		"upstream_ms", float64(d.Microseconds()) / 1000,
	}
	if st.identity != nil {
		attrs = append(attrs, "user_id", st.identity.UserID)
	}
	log := logx.From(c.Request.Context())
	if status >= http.StatusInternalServerError {
		log.Warn("gateway proxied", attrs...)
		return
	}
	log.Info("gateway proxied", attrs...)
}

// forwardedProto 返回传给上游的 X-Forwarded-Proto：仅当直连对端是受信代理时才采信入站的该头。
func (g *Gateway) forwardedProto(c *gin.Context) string {
	if g.isTrustedPeer(c.RemoteIP()) {
		if p := c.GetHeader("X-Forwarded-Proto"); p == "https" || p == "http" {
			return p
		}
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}

// applyIdentity 给已认证请求写入已签名的身份头（先剥离再写入）。
func (g *Gateway) applyIdentity(h http.Header, id authx.Identity) error {
	if err := g.signer.Apply(h, id); err != nil {
		return fmt.Errorf("sign identity headers: %w", err)
	}
	return nil
}
