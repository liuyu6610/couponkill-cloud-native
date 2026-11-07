package gateway

import (
	"math"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
)

// securityHeaders 给每个由网关处理的响应加上固定的安全头；上游自己带的同名头会被丢弃（见 modifyResponse）。
func (g *Gateway) securityHeaders(c *gin.Context) {
	h := c.Writer.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.Set("Cache-Control", "no-store")
	c.Next()
}

// guardPath 在路由之前拒绝歧义路径（点段、双斜杠、分号、反斜杠、编码变体），返回 400。
func (g *Gateway) guardPath(c *gin.Context) {
	u := c.Request.URL
	if ambiguousPath(u.EscapedPath(), u.Path) {
		g.fail(c, reasonBadPath, errBadPath)
		return
	}
	c.Next()
}

// resolveRoute 按最长前缀选上游，并决定该请求是否免登录。
// /internal/** 与未知路径一律 404：不要求登录，因此探测者无法区分「不存在」与「需要登录才能看到」。
func (g *Gateway) resolveRoute(c *gin.Context) {
	path := c.Request.URL.Path
	if isInternalPath(path) {
		g.fail(c, reasonNotFound, errNotFound)
		return
	}
	rt, ok := matchRoute(path)
	if !ok {
		g.fail(c, reasonNotFound, errNotFound)
		return
	}
	c.Set(stateKey, &reqState{route: rt, public: isPublic(c.Request.Method, path)})
	c.Next()
}

// limitBody 在声明了 Content-Length 且超限时立即返回 413；
// 分块上传（未声明长度）由 httpx 引擎的 MaxBytesReader 在转发时截断，proxyError 把它映射成 413。
func (g *Gateway) limitBody(c *gin.Context) {
	if c.Request.ContentLength > g.cfg.MaxBodyBytes {
		g.fail(c, reasonBodyTooLarge, errBodyTooLarge)
		return
	}
	c.Next()
}

// limitIP 对每个客户端 IP 做令牌桶限流（IP 取自受信代理链解析后的客户端地址）。
func (g *Gateway) limitIP(c *gin.Context) {
	g.take(c, g.ipLimit, c.ClientIP(), reasonLimitIP)
}

// limitUser 对每个已登录用户限流。免登录请求没有用户身份，只受 IP 桶约束。
func (g *Gateway) limitUser(c *gin.Context) {
	st := stateOf(c)
	if st == nil || st.identity == nil {
		c.Next()
		return
	}
	g.take(c, g.userLimit, strconv.FormatInt(st.identity.UserID, 10), reasonLimitUser)
}

// limitSeckill 对秒杀入口 POST /api/v1/order/seckill 做每用户的更严格限流。
func (g *Gateway) limitSeckill(c *gin.Context) {
	st := stateOf(c)
	if st == nil || st.identity == nil || !isSeckillEntry(c.Request.Method, c.Request.URL.Path) {
		c.Next()
		return
	}
	g.take(c, g.seckillLimit, strconv.FormatInt(st.identity.UserID, 10), reasonLimitSeckill)
}

// take 从令牌桶取一个令牌：成功则继续，否则 429 并带 Retry-After（向上取整到秒，至少 1 秒）。
func (g *Gateway) take(c *gin.Context, b *limiter.Buckets, key, reason string) {
	d := b.Take(key)
	if d.Allowed {
		c.Next()
		return
	}
	secs := int(math.Ceil(d.RetryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", strconv.Itoa(secs))
	g.fail(c, reason, apperr.RateLimited)
}
