package gateway

import "testing"

func TestMatchRoute_LongestPrefixWins(t *testing.T) {
	cases := []struct {
		path string
		want string // 上游名；空串 = 不匹配
	}{
		{"/api/v1/user/coupon/count", upstreamOrder},
		{"/api/v1/user/coupon", upstreamOrder},
		{"/api/v1/user/login", upstreamUser},
		{"/api/v1/user/profile", upstreamUser},
		{"/api/v1/user", upstreamUser},
		{"/api/v1/user/batch/generate", upstreamUser},
		{"/api/v1/auth/refresh", upstreamUser},
		{"/api/v1/coupon/list", upstreamCoupon},
		{"/api/v1/coupon/123/status", upstreamCoupon},
		{"/api/v1/order/seckill", upstreamOrder},
		{"/api/v1/order/reservations/mine", upstreamOrder},
		{"/api/v1/order/notifications/unread-count", upstreamOrder},
		{"/api/v1/connector/price-compare", upstreamConnector},
		// 前缀必须落在路径段边界上
		{"/api/v1/user/couponx", upstreamUser},
		{"/api/v1/userx/login", ""},
		{"/api/v1/couponx", ""},
		{"/api/v1/orders", ""},
		// 其他一律不匹配
		{"/", ""},
		{"/api", ""},
		{"/api/v2/user/login", ""},
		{"/order/create", ""},
		{"/internal/v1/stock/deduct", ""},
		{"/API/V1/USER/LOGIN", ""},
	}
	for _, tc := range cases {
		got, ok := matchRoute(tc.path)
		switch {
		case tc.want == "" && ok:
			t.Errorf("matchRoute(%q) = %q, want no match", tc.path, got.upstream)
		case tc.want != "" && (!ok || got.upstream != tc.want):
			t.Errorf("matchRoute(%q) = %q (ok=%v), want %q", tc.path, got.upstream, ok, tc.want)
		}
	}
}

func TestIsPublic(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/v1/user/register", true},
		{"POST", "/api/v1/user/login", true},
		{"GET", "/api/v1/user/login", false},
		{"POST", "/api/v1/user/logout", false},
		{"GET", "/api/v1/user/profile", false},
		{"GET", "/api/v1/connector/health", true},
		{"POST", "/api/v1/connector/health", false},
		{"GET", "/api/v1/connector/bindings/by-coupon/1001", true},
		{"GET", "/api/v1/connector/bindings/by-coupon/abc", false},
		{"GET", "/api/v1/connector/bindings/by-coupon/1001/extra", false},
		{"GET", "/api/v1/connector/bindings", false},
		{"GET", "/api/v1/connector/price-compare", true},
		{"GET", "/api/v1/connector/price-compare/x", false},
		{"GET", "/api/v1/connector/price-maps", false},
		{"GET", "/api/v1/connector/platforms", false},
		{"GET", "/api/v1/coupon/available", true},
		{"GET", "/api/v1/coupon/list", true},
		{"GET", "/api/v1/coupon/1001", true},
		{"GET", "/api/v1/coupon/1001/status", false},
		{"DELETE", "/api/v1/coupon/1001", false},
		{"POST", "/api/v1/coupon/available", false},
		{"POST", "/api/v1/coupon/create", false},
		{"GET", "/api/v1/coupon/create", false},
		{"GET", "/api/v1/coupon/admin/grant", false},
		{"POST", "/api/v1/order/seckill", false},
		{"GET", "/api/v1/order/user/me", false},
		{"GET", "/api/v1/auth/anything", false},
		{"GET", "/api/v1/user/coupon/count", false},
	}
	for _, tc := range cases {
		if got := isPublic(tc.method, tc.path); got != tc.want {
			t.Errorf("isPublic(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestIsSeckillEntry(t *testing.T) {
	if !isSeckillEntry("POST", "/api/v1/order/seckill") {
		t.Error("POST /api/v1/order/seckill must be the seckill entry")
	}
	for _, c := range [][2]string{
		{"GET", "/api/v1/order/seckill"},
		{"GET", "/api/v1/order/seckill/result"},
		{"POST", "/api/v1/order/create"},
		{"POST", "/api/v1/order/seckill/"},
	} {
		if isSeckillEntry(c[0], c[1]) {
			t.Errorf("%s %s must not be the seckill entry", c[0], c[1])
		}
	}
}
