package gateway_test

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 路径绕过：每一类变体都必须在到达任何上游之前被 400 拒绝，且不论是否携带有效令牌。
func TestPathGuard_EveryBypassVariantIs400BeforeRouting(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "admin")
	variants := map[string]string{
		"dot-dot":                  "/api/v1/connector/price-compare/../bindings/sync",
		"dot-dot to internal":      "/api/v1/coupon/../../internal/v1/stock/deduct",
		"single dot":               "/api/v1/coupon/./list",
		"double slash":             "/api/v1//coupon/list",
		"leading double slash":     "//api/v1/coupon/list",
		"semicolon":                "/api/v1/coupon/list;x=1",
		"semicolon mid":            "/api/v1/coupon;jsessionid=1/list",
		"backslash":                `/api/v1/coupon\list`,
		"encoded dot lower":        "/api/v1/coupon/%2e%2e/order/admin",
		"encoded dot upper":        "/api/v1/coupon/%2E%2E/order/admin",
		"encoded slash":            "/api/v1/coupon%2flist",
		"encoded slash upper":      "/api/v1/coupon%2Flist",
		"encoded backslash":        "/api/v1/coupon%5clist",
		"encoded semicolon":        "/api/v1/coupon%3blist",
		"double encoded":           "/api/v1/coupon/%252e%252e/list",
		"encoded internal prefix":  "/%69nternal/../api/v1/coupon/list",
		"encoded NUL":              "/api/v1/coupon/list%00",
		"public look-alike bypass": "/api/v1/coupon/list/..;/user/profile",
	}
	for name, target := range variants {
		for _, opts := range [][]reqOpt{nil, {bearer(token)}} {
			res := e.do("GET", target, opts...)
			if res.ResponseRecorder.Code != http.StatusBadRequest || res.Code != apperr.CodeInvalidRequest {
				t.Errorf("%s (%s) -> %d/%d, want 400/%d", name, target, res.ResponseRecorder.Code, res.Code, apperr.CodeInvalidRequest)
			}
		}
	}
	for n, u := range map[string]*fakeUpstream{"user": e.user, "coupon": e.coupon, "order": e.order, "connector": e.connector} {
		if u.Count() != 0 {
			t.Errorf("upstream %s saw %d requests from bypass variants", n, u.Count())
		}
	}
}

func TestPathGuard_OrdinaryEncodedPathsStillWork(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "admin")
	res := e.do("GET", "/api/v1/connector/probe/jd%20x/stock?sku=a%2Fb", bearer(token))
	if res.ResponseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200 for legitimately encoded paths and query strings", res.ResponseRecorder.Code)
	}
	if got := e.connector.Last(t).RawQuery; got != "sku=a%2Fb" {
		t.Errorf("query = %q, must be forwarded untouched", got)
	}
}

func TestCORS_PreflightFromAllowedOrigin(t *testing.T) {
	e := newEnv(t, nil)
	res := e.do("OPTIONS", "/api/v1/order/seckill",
		hdr("Origin", "https://app.example.com"),
		hdr("Access-Control-Request-Method", "POST"),
		hdr("Access-Control-Request-Headers", "authorization,content-type"))
	if res.ResponseRecorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", res.ResponseRecorder.Code)
	}
	h := res.Header()
	if h.Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("Allow-Origin = %q", h.Get("Access-Control-Allow-Origin"))
	}
	for _, want := range []string{"POST", "DELETE", "OPTIONS"} {
		if !strings.Contains(h.Get("Access-Control-Allow-Methods"), want) {
			t.Errorf("Allow-Methods %q lacks %s", h.Get("Access-Control-Allow-Methods"), want)
		}
	}
	if !strings.Contains(strings.ToLower(h.Get("Access-Control-Allow-Headers")), "authorization") {
		t.Errorf("Allow-Headers = %q", h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Max-Age") == "" {
		t.Error("preflight should be cacheable via Max-Age")
	}
	if h.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("credentials must stay off")
	}
	if !strings.Contains(strings.Join(h.Values("Vary"), ","), "Origin") {
		t.Errorf("Vary = %v, want Origin", h.Values("Vary"))
	}
	for n, u := range map[string]*fakeUpstream{"user": e.user, "coupon": e.coupon, "order": e.order, "connector": e.connector} {
		if u.Count() != 0 {
			t.Errorf("preflight must be answered by the gateway, upstream %s saw %d", n, u.Count())
		}
	}
}

func TestCORS_PreflightFromUnknownOriginIsRejectedWithoutCORSHeaders(t *testing.T) {
	e := newEnv(t, nil)
	res := e.do("OPTIONS", "/api/v1/order/seckill",
		hdr("Origin", "https://evil.example.com"), hdr("Access-Control-Request-Method", "POST"))
	if res.ResponseRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.ResponseRecorder.Code)
	}
	if res.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("an unknown origin must get no Allow-Origin")
	}
}

func TestCORS_ActualResponsesCarryAllowOriginIncludingGatewayErrors(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "1"})
	origin := hdr("Origin", "https://app.example.com")

	ok := e.do("GET", "/api/v1/coupon/list", origin)
	if ok.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("200 response Allow-Origin = %q", ok.Header().Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(ok.Header().Get("Access-Control-Expose-Headers"), "X-Request-Id") {
		t.Errorf("Expose-Headers = %q", ok.Header().Get("Access-Control-Expose-Headers"))
	}
	limited := e.do("GET", "/api/v1/coupon/list", origin)
	if limited.ResponseRecorder.Code != 429 || limited.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Errorf("429 must be readable cross-origin: status %d allow-origin %q", limited.ResponseRecorder.Code, limited.Header().Get("Access-Control-Allow-Origin"))
	}

	e2 := newEnv(t, nil)
	denied := e2.do("GET", "/api/v1/coupon/list", hdr("Origin", "https://evil.example.com"))
	if denied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("an unknown origin must not get Allow-Origin on normal responses either")
	}
	noOrigin := e2.do("GET", "/api/v1/coupon/list")
	if noOrigin.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("requests without Origin get no CORS headers")
	}
}

func TestCORS_UpstreamCORSHeadersDoNotDuplicate(t *testing.T) {
	e := newEnv(t, nil)
	e.coupon.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{}}`)
	})
	res := e.do("GET", "/api/v1/coupon/list", hdr("Origin", "https://app.example.com"))
	if v := res.Header().Values("Access-Control-Allow-Origin"); len(v) != 1 || v[0] != "https://app.example.com" {
		t.Errorf("Allow-Origin = %v, want only the gateway's value", v)
	}
	if v := res.Header().Values("X-Frame-Options"); len(v) != 1 || v[0] != "DENY" {
		t.Errorf("X-Frame-Options = %v, want only the gateway's DENY", v)
	}
}

func TestSecurityHeadersOnGatewayResponses(t *testing.T) {
	e := newEnv(t, nil)
	for _, res := range []*result{
		e.do("GET", "/api/v1/coupon/list"),   // 代理成功
		e.do("GET", "/nope"),                 // 404
		e.do("GET", "/api/v1/order/user/me"), // 401
	} {
		h := res.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" ||
			h.Get("Referrer-Policy") != "no-referrer" || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("status %d is missing security headers: %v", res.ResponseRecorder.Code, h)
		}
		if h.Get("X-Request-Id") == "" {
			t.Errorf("status %d has no X-Request-Id", res.ResponseRecorder.Code)
		}
	}
}

func TestBodyLimit_DeclaredLengthOverLimitIs413Envelope(t *testing.T) {
	e := newEnv(t, nil) // 上限 4096
	token, _ := e.login(1, "user")
	res := e.do("POST", "/api/v1/order/create", bearer(token), body(strings.Repeat("x", 5000)))
	if res.ResponseRecorder.Code != http.StatusRequestEntityTooLarge || res.Code != apperr.CodeInvalidRequest {
		t.Fatalf("status/code %d/%d, want 413/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeInvalidRequest)
	}
	if e.order.Count() != 0 {
		t.Fatal("an oversize body must not be forwarded")
	}
	if res := e.do("POST", "/api/v1/order/create", bearer(token), body(strings.Repeat("x", 4096))); res.ResponseRecorder.Code != 200 {
		t.Errorf("a body exactly at the limit: status %d, want 200", res.ResponseRecorder.Code)
	}
}

func TestBodyLimit_ChunkedBodyOverLimitIs413AndDoesNotTripTheBreaker(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "user")
	srv := e.serve()

	post := func(payload string) *http.Response {
		req, err := http.NewRequest("POST", srv.URL+"/api/v1/order/create", io.NopCloser(strings.NewReader(payload))) // ContentLength 未知 → 分块
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	// 超过熔断阈值（3）多次：413 是客户端的错，不能让上游被熔断。
	for i := 0; i < 5; i++ {
		resp := post(strings.Repeat("y", 10000))
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(b), `"code":10004`) {
			t.Fatalf("chunked oversize %d: status %d body %s, want a 413 envelope", i, resp.StatusCode, b)
		}
	}
	resp := post("small")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("a small chunked body afterwards: status %d, want 200 (breaker must not have opened)", resp.StatusCode)
	}
}

func TestStreaming_LargeResponsePassesThroughIntact(t *testing.T) {
	e := newEnv(t, nil)
	payload := bytes.Repeat([]byte("0123456789abcdef"), 1<<20) // 16 MiB
	want := sha256.Sum256(payload)
	e.coupon.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		for off := 0; off < len(payload); off += 64 << 10 {
			_, _ = w.Write(payload[off : off+(64<<10)])
		}
	})
	srv := e.serve()
	resp, err := http.Get(srv.URL + "/api/v1/coupon/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	h := sha256.New()
	n, err := io.Copy(h, resp.Body)
	if err != nil || n != int64(len(payload)) {
		t.Fatalf("read %d bytes, err %v, want %d", n, err, len(payload))
	}
	var got [32]byte
	copy(got[:], h.Sum(nil))
	if got != want {
		t.Fatal("the response body was altered in transit")
	}
}

func TestStreaming_FirstChunkArrivesBeforeTheUpstreamFinishes(t *testing.T) {
	e := newEnv(t, nil)
	release := make(chan struct{})
	e.coupon.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "data: two\n\n")
	})
	srv := e.serve()
	resp, err := http.Get(srv.URL + "/api/v1/coupon/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, len("data: one\n\n"))
	done := make(chan error, 1)
	go func() { _, err := io.ReadFull(resp.Body, buf); done <- err }()
	select {
	case err := <-done:
		if err != nil || string(buf) != "data: one\n\n" {
			t.Fatalf("first chunk = %q, err %v", buf, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the gateway buffered the stream instead of passing it through")
	}
	close(release)
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "data: two\n\n" {
		t.Errorf("rest = %q", rest)
	}
}

func TestStreaming_LargeRequestBodyWithinLimitIsForwardedIntact(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_MAX_BODY_BYTES": "2097152"})
	token, _ := e.login(1, "user")
	payload := bytes.Repeat([]byte("abcdefgh"), 1<<17) // 1 MiB
	srv := e.serve()
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/order/create", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := e.order.Last(t).Body; !bytes.Equal(got, payload) {
		t.Fatalf("upstream received %d bytes, want %d identical bytes", len(got), len(payload))
	}
}

func TestHealth_HealthzIsPublicAndReadyzReflectsRedis(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "1"})
	for i := 0; i < 5; i++ { // 探针不受 IP 限流影响，也不需要登录
		if res := e.do("GET", "/healthz"); res.ResponseRecorder.Code != 200 {
			t.Fatalf("healthz #%d: status %d", i, res.ResponseRecorder.Code)
		}
	}
	if res := e.do("GET", "/readyz"); res.ResponseRecorder.Code != 200 {
		t.Fatalf("readyz with healthy redis: %d", res.ResponseRecorder.Code)
	}
	e.mr.Close()
	if res := e.do("GET", "/readyz"); res.ResponseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz with redis down and revocation enabled: %d, want 503", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/healthz"); res.ResponseRecorder.Code != 200 {
		t.Fatalf("healthz must stay up when redis is down: %d", res.ResponseRecorder.Code)
	}
}

func TestHealth_ReadyzIgnoresRedisWhenRevocationDisabled(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_REVOCATION_ENABLED": "false"})
	e.mr.Close()
	if res := e.do("GET", "/readyz"); res.ResponseRecorder.Code != 200 {
		t.Fatalf("readyz = %d, want 200: redis is not a dependency when revocation is off", res.ResponseRecorder.Code)
	}
}

func TestHealth_ProbesStripSpoofedIdentityToo(t *testing.T) {
	e := newEnv(t, nil)
	res := e.do("GET", "/healthz", hdr("X-User-Id", "1"))
	if res.ResponseRecorder.Code != 200 {
		t.Fatalf("status %d", res.ResponseRecorder.Code)
	}
}

func TestMetrics_RecordProxiedRejectedAndBreakerSeries(t *testing.T) {
	e := newEnv(t, nil)
	e.do("GET", "/api/v1/coupon/list")
	e.do("GET", "/api/v1/order/user/me") // 401
	e.do("GET", "/internal/v1/x")        // 404
	res := e.do("GET", "/metrics")
	if res.ResponseRecorder.Code != 200 {
		t.Fatalf("metrics status %d", res.ResponseRecorder.Code)
	}
	text := res.Body.String()
	for _, want := range []string{
		`ck_gateway_proxied_total{service="gateway",status="200",upstream="coupon"} 1`,
		`ck_gateway_rejected_total{reason="unauthenticated",service="gateway"} 1`,
		`ck_gateway_rejected_total{reason="not_found",service="gateway"} 1`,
		`ck_gateway_breaker_state{service="gateway",upstream="order"} 0`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}
