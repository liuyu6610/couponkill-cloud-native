package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// corsPolicy 是跨域白名单策略：精确匹配源，不允许携带凭据（Cookie），令牌走 Authorization 头。
type corsPolicy struct {
	origins map[string]struct{}
}

// 预检响应里声明的允许项。
const (
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeaders = "Authorization, Content-Type, X-Request-Id"
	corsExposeHeader = "X-Request-Id, Retry-After"
	corsMaxAge       = "600"
)

func newCORSPolicy(origins []string) corsPolicy {
	p := corsPolicy{origins: make(map[string]struct{}, len(origins))}
	for _, o := range origins {
		p.origins[strings.ToLower(o)] = struct{}{}
	}
	return p
}

func (p corsPolicy) allowed(origin string) bool {
	_, ok := p.origins[strings.ToLower(origin)]
	return ok
}

// handleCORS 处理跨域：白名单内的源在所有响应（包括 401/429 等网关拒绝）上带 Allow-Origin，
// 预检请求（OPTIONS + Access-Control-Request-Method）在这里直接以 204 结束，不鉴权、不转发、不占限流额度。
// 不在白名单内的源得不到任何 CORS 头；对它的预检返回 403。
func (g *Gateway) handleCORS(c *gin.Context) {
	origin := c.GetHeader("Origin")
	if origin == "" {
		c.Next()
		return
	}
	h := c.Writer.Header()
	h.Add("Vary", "Origin")
	preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
	if !g.cors.allowed(origin) {
		if preflight {
			g.fail(c, reasonCORS, apperr.Forbidden.WithMessage("跨域来源不被允许"))
			return
		}
		c.Next()
		return
	}
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Expose-Headers", corsExposeHeader)
	if preflight {
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		h.Set("Access-Control-Allow-Methods", corsAllowMethods)
		h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
		h.Set("Access-Control-Max-Age", corsMaxAge)
		c.AbortWithStatus(http.StatusNoContent)
		return
	}
	c.Next()
}
