package gateway_test

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// flappy 让假上游按开关返回 500 或 200，模拟时好时坏的服务。
type flappy struct{ failing atomic.Bool }

func (f *flappy) handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if f.failing.Load() {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":50000,"message":"系统错误","data":null}`)
		return
	}
	_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{}}`)
}

func TestBreaker_OpensAfterConsecutiveFailuresThenRecoversViaHalfOpen(t *testing.T) {
	e := newEnv(t, nil) // 阈值 3，打开 30s
	token, _ := e.login(1, "user")
	f := &flappy{}
	f.failing.Store(true)
	e.order.Handle(f.handler)

	// 前 3 次 500 原样透传并计入失败。
	for i := 0; i < 3; i++ {
		res := e.do("GET", "/api/v1/order/user/me", bearer(token))
		if res.ResponseRecorder.Code != 500 || res.Code != apperr.CodeSysError {
			t.Fatalf("failure %d: status/code %d/%d, want the upstream 500 passed through", i, res.ResponseRecorder.Code, res.Code)
		}
	}
	// 熔断器打开：快速失败 503，不再打到上游。
	before := e.order.Count()
	res := e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != http.StatusServiceUnavailable || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("open breaker: status/code %d/%d, want 503/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeSystemBusy)
	}
	if e.order.Count() != before {
		t.Fatal("an open breaker must not call the upstream")
	}
	// 熔断是按上游隔离的：别的上游不受影响。
	if res := e.do("GET", "/api/v1/coupon/list"); res.ResponseRecorder.Code != 200 {
		t.Errorf("coupon upstream affected by the order breaker: %d", res.ResponseRecorder.Code)
	}

	// 半开：超时后放行一个探测请求；探测仍失败 → 重新打开。
	e.clock.Advance(31 * time.Second)
	res = e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != 500 {
		t.Fatalf("half-open probe should reach the upstream (still failing), got %d", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 503 {
		t.Fatalf("failed probe must re-open the breaker, got %d", res.ResponseRecorder.Code)
	}

	// 再次半开，上游恢复：探测成功 → 关闭，流量恢复。
	f.failing.Store(false)
	e.clock.Advance(31 * time.Second)
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Fatalf("successful probe: status %d, want 200", res.ResponseRecorder.Code)
	}
	for i := 0; i < 5; i++ {
		if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
			t.Fatalf("breaker should be closed again, request %d got %d", i, res.ResponseRecorder.Code)
		}
	}
}

func TestBreaker_CouponUpstreamUsesItsOwnUnavailableCode(t *testing.T) {
	e := newEnv(t, nil)
	f := &flappy{}
	f.failing.Store(true)
	e.coupon.Handle(f.handler)
	for i := 0; i < 3; i++ {
		e.do("GET", "/api/v1/coupon/list")
	}
	res := e.do("GET", "/api/v1/coupon/list")
	if res.ResponseRecorder.Code != 503 || res.Code != apperr.CodeCouponServiceUnavailable {
		t.Fatalf("status/code %d/%d, want 503/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeCouponServiceUnavailable)
	}
}

func TestBreaker_ClientErrorsNeverCountAsFailures(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "user")
	e.order.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":10002,"message":"库存不足","data":null}`)
	})
	for i := 0; i < 20; i++ {
		if res := e.do("POST", "/api/v1/order/create", bearer(token)); res.ResponseRecorder.Code != 409 {
			t.Fatalf("request %d: status %d, want the upstream 409 (breaker must stay closed)", i, res.ResponseRecorder.Code)
		}
	}
	e.order.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	for i := 0; i < 20; i++ {
		if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 404 {
			t.Fatalf("404 request %d: status %d", i, res.ResponseRecorder.Code)
		}
	}
}

func TestBreaker_SuccessInterleavedResetsTheFailureStreak(t *testing.T) {
	e := newEnv(t, nil) // 阈值 3：需要「连续」失败
	token, _ := e.login(1, "user")
	f := &flappy{}
	e.order.Handle(f.handler)
	for round := 0; round < 4; round++ {
		f.failing.Store(true)
		e.do("GET", "/api/v1/order/user/me", bearer(token))
		e.do("GET", "/api/v1/order/user/me", bearer(token))
		f.failing.Store(false)
		if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
			t.Fatalf("round %d: breaker opened on non-consecutive failures (status %d)", round, res.ResponseRecorder.Code)
		}
	}
}

func TestBreaker_TransportErrorsCountAsFailures(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "user")
	e.order.Close()
	for i := 0; i < 3; i++ {
		if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != http.StatusBadGateway {
			t.Fatalf("transport failure %d: status %d, want 502", i, res.ResponseRecorder.Code)
		}
	}
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("after 3 transport failures the breaker must be open, got %d", res.ResponseRecorder.Code)
	}
}

func TestTimeout_SlowUpstreamGets504EnvelopeAndCountsAsFailure(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_UPSTREAM_TIMEOUT": "80ms", "CK_GATEWAY_BREAKER_FAILURES": "2"})
	token, _ := e.login(1, "user")
	e.order.Handle(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, `{"code":200}`)
	})

	start := time.Now()
	res := e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != http.StatusGatewayTimeout || res.Code != apperr.CodeSystemBusy {
		t.Fatalf("status/code %d/%d, want 504/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeSystemBusy)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v, the 80ms timeout was not enforced", d)
	}
	if strings.Contains(res.Body.String(), "deadline") || strings.Contains(res.Body.String(), "127.0.0.1") {
		t.Errorf("timeout response leaks internals: %s", res.Body.String())
	}
	// 第二次超时后，熔断器（阈值 2）打开。
	e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != http.StatusServiceUnavailable {
		t.Errorf("timeouts must trip the breaker, got %d", res.ResponseRecorder.Code)
	}
}

func TestTimeout_SeckillEntryUsesTheShorterTimeout(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_UPSTREAM_TIMEOUT": "1s", "CK_GATEWAY_SECKILL_TIMEOUT": "60ms"})
	token, _ := e.login(1, "user")
	e.order.Handle(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{}}`)
	})
	if res := e.do("POST", "/api/v1/order/seckill", bearer(token)); res.ResponseRecorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("seckill: status %d, want 504 after the short seckill timeout", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/order/seckill/result?requestId=x", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Fatalf("result polling must use the normal timeout, got %d", res.ResponseRecorder.Code)
	}
}

func TestTimeout_ConnectorHasItsOwnLongerTimeout(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_UPSTREAM_TIMEOUT": "60ms", "CK_GATEWAY_CONNECTOR_TIMEOUT": "1s"})
	token, _ := e.login(1, "admin")
	slow := func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{"code":200,"message":"ok","data":{}}`)
	}
	e.connector.Handle(slow)
	e.coupon.Handle(slow)
	if res := e.do("POST", "/api/v1/connector/sync", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Errorf("connector sync: status %d, want 200 under the connector timeout", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/coupon/list"); res.ResponseRecorder.Code != http.StatusGatewayTimeout {
		t.Errorf("coupon: status %d, want 504 under the default timeout", res.ResponseRecorder.Code)
	}
}

func TestTimeout_OnlyBoundsTimeToResponseHeaders(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_UPSTREAM_TIMEOUT": "100ms"})
	e.coupon.Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "first|")
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond) // 响应体传输时间超过上游超时
		_, _ = io.WriteString(w, "second")
	})
	srv := e.serve()
	resp, err := http.Get(srv.URL + "/api/v1/coupon/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil || string(b) != "first|second" {
		t.Fatalf("body = %q, err = %v: a slow body must not be cut by the header timeout", b, err)
	}
}
