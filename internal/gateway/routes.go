package gateway

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// 上游名称：既是路由表的目标，也是熔断器、指标与日志里的标签（基数固定为 4）。
const (
	upstreamUser      = "user"
	upstreamCoupon    = "coupon"
	upstreamOrder     = "order"
	upstreamConnector = "connector"
)

// seckillPath 是秒杀入口路径：单独限流并使用更短的上游超时。
const seckillPath = "/api/v1/order/seckill"

// route 是路由表的一项：路径前缀（按路径段边界匹配）到上游。
type route struct {
	prefix   string
	upstream string
}

// routeTable 按前缀长度降序排列，因此第一个命中的就是最长前缀（设计 §6）。
var routeTable = sortedRoutes([]route{
	{"/api/v1/user/coupon", upstreamOrder},
	{"/api/v1/user", upstreamUser},
	{"/api/v1/auth", upstreamUser},
	{"/api/v1/coupon", upstreamCoupon},
	{"/api/v1/order", upstreamOrder},
	{"/api/v1/connector", upstreamConnector},
})

func sortedRoutes(rs []route) []route {
	slices.SortStableFunc(rs, func(a, b route) int { return len(b.prefix) - len(a.prefix) })
	return rs
}

// matchRoute 返回最长前缀命中的路由。前缀必须落在路径段边界上：
// "/api/v1/user/coupon" 命中 "/api/v1/user/coupon/count"，但不命中 "/api/v1/user/couponx"。
func matchRoute(path string) (route, bool) {
	for _, r := range routeTable {
		if path == r.prefix || strings.HasPrefix(path, r.prefix+"/") {
			return r, true
		}
	}
	return route{}, false
}

// isInternalPath 报告路径是否属于服务间内部接口（/internal/**），网关对它们一律返回 404。
func isInternalPath(path string) bool {
	return path == "/internal" || strings.HasPrefix(path, "/internal/")
}

// 免登录的只读路径形状。券 ID 是纯数字（snowflake），其余形状一律需要登录。
var (
	publicCouponRead   = regexp.MustCompile(`^/api/v1/coupon/(available|list|[0-9]{1,19})$`)
	publicBindingRead  = regexp.MustCompile(`^/api/v1/connector/bindings/by-coupon/[0-9]{1,19}$`)
	publicPriceCompare = "/api/v1/connector/price-compare"
	publicConnectorHlt = "/api/v1/connector/health"
)

// isPublic 报告 (method, path) 是否在免登录白名单内（设计 §6，依据前端实际调用裁剪）：
//
//   - POST /api/v1/user/register、/api/v1/user/login：登录前必须可达；
//   - GET /api/v1/coupon/available|list|{id}：首页、券列表、券详情、秒杀页均无需登录（App.tsx 中未加 ProtectedRoute）；
//   - GET /api/v1/connector/health、/bindings/by-coupon/{id}、/price-compare：券详情页的只读展示。
//
// 其余路径（包括前端未使用的任何新增接口）默认需要登录：宁严勿松。
func isPublic(method, path string) bool {
	switch method {
	case http.MethodPost:
		return path == "/api/v1/user/register" || path == "/api/v1/user/login"
	case http.MethodGet:
		return publicCouponRead.MatchString(path) ||
			publicBindingRead.MatchString(path) ||
			path == publicPriceCompare ||
			path == publicConnectorHlt
	}
	return false
}

// isSeckillEntry 报告请求是否是秒杀入口 POST /api/v1/order/seckill（轮询结果的 GET 不算）。
func isSeckillEntry(method, path string) bool {
	return method == http.MethodPost && path == seckillPath
}
