package gateway_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 每个路由前缀都命中正确的上游，并且上游收到的是网关签名的身份头。
func TestRouting_EveryPrefixHitsTheRightUpstream(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(4242, "user", "admin")

	cases := []struct {
		method, path string
		want         string
	}{
		{"GET", "/api/v1/user/profile", "user"},
		{"POST", "/api/v1/user/logout", "user"},
		{"POST", "/api/v1/user/batch/generate", "user"},
		{"GET", "/api/v1/auth/whatever", "user"},
		{"GET", "/api/v1/user/coupon/count", "order"},
		{"GET", "/api/v1/coupon/list/extra", "coupon"},
		{"POST", "/api/v1/coupon/create", "coupon"},
		{"DELETE", "/api/v1/coupon/1001", "coupon"},
		{"POST", "/api/v1/order/seckill", "order"},
		{"GET", "/api/v1/order/reservations/mine", "order"},
		{"POST", "/api/v1/order/notifications/read-all", "order"},
		{"GET", "/api/v1/connector/bindings", "connector"},
		{"POST", "/api/v1/connector/sync", "connector"},
	}
	ups := map[string]*fakeUpstream{"user": e.user, "coupon": e.coupon, "order": e.order, "connector": e.connector}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			before := map[string]int{}
			for n, u := range ups {
				before[n] = u.Count()
			}
			res := e.do(tc.method, tc.path, bearer(token))
			if res.Code != http.StatusOK || res.ResponseRecorder.Code != http.StatusOK {
				t.Fatalf("status = %d code = %d body = %s", res.ResponseRecorder.Code, res.Code, res.Body.String())
			}
			for n, u := range ups {
				got := u.Count() - before[n]
				switch {
				case n == tc.want && got != 1:
					t.Errorf("upstream %s saw %d requests, want 1", n, got)
				case n != tc.want && got != 0:
					t.Errorf("upstream %s saw %d requests, want 0", n, got)
				}
			}
			last := ups[tc.want].Last(t)
			if last.Method != tc.method || last.Path != tc.path {
				t.Errorf("upstream got %s %s, want %s %s", last.Method, last.Path, tc.method, tc.path)
			}
			id := verifyIdentity(t, last.Header, e.clock.Now())
			if id.UserID != 4242 || !id.HasRole("admin") || !id.HasRole("user") {
				t.Errorf("identity = %+v", id)
			}
		})
	}
}

func TestRouting_ForwardsMethodQueryBodyAndAuthorization(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(7, "user")

	res := e.do("POST", "/api/v1/order/create?couponId=1002&x=a%20b", bearer(token), body(`{"k":"v"}`), hdr("Content-Type", "application/json"))
	if res.ResponseRecorder.Code != 200 {
		t.Fatalf("status = %d: %s", res.ResponseRecorder.Code, res.Body.String())
	}
	got := e.order.Last(t)
	if got.RawQuery != "couponId=1002&x=a%20b" {
		t.Errorf("query = %q", got.RawQuery)
	}
	if string(got.Body) != `{"k":"v"}` {
		t.Errorf("body = %q", got.Body)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("content-type = %q", got.Header.Get("Content-Type"))
	}
	// 登出需要 jti，user 服务要自己解析令牌，所以 Authorization 必须原样转发。
	if got.Header.Get("Authorization") != "Bearer "+token {
		t.Errorf("Authorization was not forwarded unchanged: %q", got.Header.Get("Authorization"))
	}
}

func TestRouting_PreservesUpstreamStatusAndHeaders(t *testing.T) {
	e := newEnv(t, nil)
	e.coupon.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "a=b; Path=/")
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":10002,"message":"库存不足","data":null}`))
	})
	res := e.do("GET", "/api/v1/coupon/list")
	if res.ResponseRecorder.Code != http.StatusConflict || res.Code != apperr.CodeOutOfStock {
		t.Fatalf("status/code = %d/%d, want 409/10002 passed through", res.ResponseRecorder.Code, res.Code)
	}
	if res.Header().Get("X-Custom") != "yes" || res.Header().Get("Set-Cookie") != "a=b; Path=/" {
		t.Errorf("upstream headers lost: %v", res.Header())
	}
}

func TestRouting_RequestIDPropagatedAndForwardedHeadersSet(t *testing.T) {
	e := newEnv(t, nil)

	res := e.do("GET", "/api/v1/coupon/list", hdr("X-Request-Id", "trace-abc-123"), hdr("X-Forwarded-For", "6.6.6.6"), hdr("Host", "api.example.com"))
	got := e.coupon.Last(t)
	if got.Header.Get("X-Request-Id") != "trace-abc-123" {
		t.Errorf("upstream X-Request-Id = %q, want it propagated", got.Header.Get("X-Request-Id"))
	}
	if ids := res.Header().Values("X-Request-Id"); len(ids) != 1 || ids[0] != "trace-abc-123" {
		t.Errorf("response X-Request-Id = %v, want exactly the propagated one", ids)
	}
	// 默认不信任代理：客户端伪造的 X-Forwarded-For 不能传给上游，上游看到的是真实对端。
	if xff := got.Header.Get("X-Forwarded-For"); xff != "203.0.113.7" {
		t.Errorf("X-Forwarded-For = %q, want the real peer 203.0.113.7", xff)
	}
	if got.Header.Get("X-Forwarded-Proto") != "http" {
		t.Errorf("X-Forwarded-Proto = %q", got.Header.Get("X-Forwarded-Proto"))
	}
}

func TestRouting_GeneratesRequestIDWhenMissing(t *testing.T) {
	e := newEnv(t, nil)
	e.do("GET", "/api/v1/coupon/list")
	if e.coupon.Last(t).Header.Get("X-Request-Id") == "" {
		t.Error("gateway must generate and propagate a request id")
	}
}

func TestRouting_InternalPathsAre404WithoutReachingAnyUpstream(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "admin")
	for _, p := range []string{
		"/internal/v1/stock/deduct",
		"/internal/v1/orders/grant",
		"/internal/",
		"/internal",
		"/internal/v1/users/1",
	} {
		for _, opts := range [][]reqOpt{nil, {bearer(token)}, {hdr("X-Internal-Token", "anything")}} {
			res := e.do("POST", p, opts...)
			if res.ResponseRecorder.Code != http.StatusNotFound || res.Code != apperr.CodeInvalidRequest {
				t.Errorf("POST %s -> %d/%d, want 404 envelope", p, res.ResponseRecorder.Code, res.Code)
			}
		}
	}
	for n, u := range map[string]*fakeUpstream{"user": e.user, "coupon": e.coupon, "order": e.order, "connector": e.connector} {
		if u.Count() != 0 {
			t.Errorf("upstream %s must not see /internal/** traffic, saw %d", n, u.Count())
		}
	}
}

func TestRouting_UnknownPathsAre404Envelope(t *testing.T) {
	e := newEnv(t, nil)
	for _, p := range []string{"/", "/api", "/api/v1", "/api/v2/user/login", "/order/create", "/api/v1/userx", "/admin", "/api/v1/user-coupon"} {
		res := e.do("GET", p)
		if res.ResponseRecorder.Code != http.StatusNotFound || res.Code != apperr.CodeInvalidRequest {
			t.Errorf("GET %s -> %d/%d, want 404 envelope", p, res.ResponseRecorder.Code, res.Code)
		}
		if !strings.Contains(res.Body.String(), `"data":null`) {
			t.Errorf("GET %s body is not an envelope: %s", p, res.Body.String())
		}
	}
}

func TestRouting_UserCouponPrefixBeatsUserPrefix(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(9)
	e.do("GET", "/api/v1/user/coupon/count", bearer(token))
	if e.order.Count() != 1 || e.user.Count() != 0 {
		t.Fatalf("order=%d user=%d, want the longer /user/coupon prefix to win", e.order.Count(), e.user.Count())
	}
	e.do("GET", "/api/v1/user/profile", bearer(token))
	if e.user.Count() != 1 {
		t.Fatalf("user=%d, want /user/profile on user", e.user.Count())
	}
}

func TestRouting_UnreachableUpstreamIs502EnvelopeWithoutLeakingDetails(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(5)
	e.order.Close() // 连接会被拒绝

	res := e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != http.StatusBadGateway || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("status/code = %d/%d, want 502/50003", res.ResponseRecorder.Code, res.Code)
	}
	for _, leak := range []string{"127.0.0.1", "connection refused", "dial", e.order.URL} {
		if strings.Contains(res.Body.String(), leak) {
			t.Errorf("response leaks upstream detail %q: %s", leak, res.Body.String())
		}
	}
	if !strings.Contains(res.Body.String(), `"requestId"`) {
		t.Errorf("error envelope should carry the request id: %s", res.Body.String())
	}
}

func TestRouting_CouponUpstreamFailureUsesCouponUnavailableCode(t *testing.T) {
	e := newEnv(t, nil)
	e.coupon.Close()
	res := e.do("GET", "/api/v1/coupon/list")
	if res.ResponseRecorder.Code != http.StatusBadGateway || res.Code != apperr.CodeCouponServiceUnavailable {
		t.Fatalf("status/code = %d/%d, want 502/10013", res.ResponseRecorder.Code, res.Code)
	}
}

func TestRouting_StripsIdentityBeforeAnything(t *testing.T) {
	// 未经 Handler 入口剥离的头不应出现在上游（public 路径）。
	e := newEnv(t, nil)
	e.do("GET", "/api/v1/coupon/list",
		hdr("X-User-Id", "1"), hdr("x-user-roles", "admin"), hdr("X-Authenticated", "true"),
		hdr("X-Gateway-Sig", "deadbeef"), hdr("X-Gateway-Ts", "1"), hdr("X-Gateway-Extra", "x"))
	got := e.coupon.Last(t)
	for _, h := range []string{"X-User-Id", "X-User-Roles", "X-Authenticated", "X-Gateway-Sig", "X-Gateway-Ts", "X-Gateway-Extra"} {
		if v := got.Header.Get(h); v != "" {
			t.Errorf("public request leaked spoofed %s=%q to the upstream", h, v)
		}
	}
}
