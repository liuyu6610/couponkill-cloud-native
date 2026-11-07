package gateway

import (
	"net/url"
	"testing"
)

// 每个绕过变体都对应 Java InternalApiBlockFilter 里的一条拦截意图：
// 网关按原样转发路径，下游若再做一次归一化/解码，前缀白名单就会和实际命中的接口不一致。
func TestAmbiguousPath_RejectsEveryBypassVariant(t *testing.T) {
	bad := map[string]string{
		"dot-dot segment":          "/api/v1/coupon/../order/admin",
		"dot-dot at end":           "/api/v1/coupon/..",
		"single dot segment":       "/api/v1/coupon/./list",
		"double slash inside":      "/api/v1//coupon/list",
		"double slash at start":    "//api/v1/coupon/list",
		"semicolon path param":     "/api/v1/coupon;jsessionid=1/list",
		"semicolon at end":         "/api/v1/coupon/list;",
		"backslash":                `/api/v1/coupon\..\order/admin`,
		"encoded dot lower":        "/api/v1/coupon/%2e%2e/order",
		"encoded dot upper":        "/api/v1/coupon/%2E%2E/order",
		"half encoded dot":         "/api/v1/coupon/.%2e/order",
		"encoded slash lower":      "/api/v1/coupon%2flist",
		"encoded slash upper":      "/api/v1/coupon%2Flist",
		"encoded backslash lower":  "/api/v1/coupon%5clist",
		"encoded backslash upper":  "/api/v1/coupon%5Clist",
		"encoded semicolon lower":  "/api/v1/coupon%3blist",
		"encoded semicolon upper":  "/api/v1/coupon%3Blist",
		"encoded percent (double)": "/api/v1/coupon/%252e%252e/order",
		"bare encoded percent":     "/api/v1/coupon/100%25",
		"encoded nul":              "/api/v1/coupon/list%00.png",
		"encoded newline":          "/api/v1/coupon/list%0a",
		"encoded tab":              "/api/v1/coupon/list%09",
		"encoded del":              "/api/v1/coupon/list%7f",
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			u, err := url.ParseRequestURI(raw) // 与服务器解析请求行的方式一致
			if err != nil {
				t.Fatalf("test fixture does not parse: %v", err)
			}
			if !ambiguousPath(u.EscapedPath(), u.Path) {
				t.Errorf("ambiguousPath(%q) = false, want true", raw)
			}
		})
	}
}

func TestAmbiguousPath_AllowsOrdinaryPaths(t *testing.T) {
	good := []string{
		"/api/v1/coupon/list",
		"/api/v1/coupon/1001",
		"/api/v1/connector/probe/jd/stock",
		"/api/v1/order/seckill/result",
		"/api/v1/coupon/%E4%BC%98%E6%83%A0", // 合法的 UTF-8 百分号编码
		"/api/v1/coupon/a%20b",
		"/api/v1/coupon/a..b", // 点出现在段内部不是点段
		"/api/v1/coupon/.hidden",
		"/api/v1/coupon/list/",
		"/",
	}
	for _, raw := range good {
		u, err := url.ParseRequestURI(raw) // 与服务器解析请求行的方式一致
		if err != nil {
			t.Fatalf("fixture %q: %v", raw, err)
		}
		if ambiguousPath(u.EscapedPath(), u.Path) {
			t.Errorf("ambiguousPath(%q) = true, want false", raw)
		}
	}
}
