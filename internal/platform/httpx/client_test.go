package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

type deductReq struct {
	RequestID string `json:"requestId"`
	CouponID  string `json:"couponId"`
}

type deductResp struct {
	Bucket int `json:"bucket"`
}

func ok(w http.ResponseWriter, data string) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":` + data + `}`))
}

func biz(w http.ResponseWriter, status, code int, msg string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"code":` + itoa(code) + `,"message":"` + msg + `","data":null}`))
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newClient(t *testing.T, h http.Handler, mut ...func(*httpx.ClientOptions)) *httpx.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts := httpx.ClientOptions{
		BaseURL: srv.URL, Timeout: time.Second, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond,
		InternalToken: testkit.InternalToken,
	}
	for _, m := range mut {
		m(&opts)
	}
	return httpx.NewClient(opts)
}

func TestClientDecodesEnvelopeAndSendsHeadersAndBody(t *testing.T) {
	var gotReq deductReq
	var gotHeader http.Header
	var gotMethod, gotPath, gotCT string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader, gotMethod, gotPath, gotCT = r.Header.Clone(), r.Method, r.URL.Path, r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		ok(w, `{"bucket":3}`)
	}))
	ctx := logx.WithRequestID(context.Background(), "req-77")
	var out deductResp
	if err := c.Post(ctx, "/internal/v1/stock/deduct", deductReq{"r1", "1001"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Bucket != 3 || gotReq.RequestID != "r1" || gotMethod != "POST" || gotPath != "/internal/v1/stock/deduct" || gotCT != "application/json" {
		t.Fatalf("round trip wrong: out=%+v req=%+v %s %s %s", out, gotReq, gotMethod, gotPath, gotCT)
	}
	if gotHeader.Get("X-Request-Id") != "req-77" {
		t.Fatalf("request id must propagate, got %q", gotHeader.Get("X-Request-Id"))
	}
	if gotHeader.Get("X-Internal-Token") != testkit.InternalToken {
		t.Fatal("internal token header missing")
	}
}

func TestClientGeneratesRequestIDWhenContextHasNone(t *testing.T) {
	var id string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { id = r.Header.Get("X-Request-Id"); ok(w, `null`) }))
	if err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("a request id must always be sent, got %q", id)
	}
}

func TestClientSignedIdentityHeaders(t *testing.T) {
	var h http.Header
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h = r.Header.Clone(); ok(w, `null`) }))
	signer := testkit.Signer(t)
	if err := c.Get(context.Background(), "/x", nil, httpx.AsIdentity(signer, authx.Identity{UserID: 5, Roles: []string{"admin"}}), httpx.Header("X-Custom", "1")); err != nil {
		t.Fatal(err)
	}
	id, err := signer.Verify(h)
	if err != nil || id.UserID != 5 || !id.IsAdmin() {
		t.Fatalf("outbound identity must verify at the receiver: %+v %v", id, err)
	}
	if h.Get("X-Custom") != "1" {
		t.Fatal("custom header lost")
	}
}

func TestClientRestoresBusinessErrorsFromEnvelope(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oos":
			biz(w, 409, apperr.CodeOutOfStock, "库存不足")
		case "/notfound":
			biz(w, 404, apperr.CodeCouponNotFound, "优惠券不存在")
		case "/ok-status-bad-code":
			biz(w, 200, apperr.CodeSysError, "weird")
		}
	}))
	err := c.Post(context.Background(), "/oos", nil, nil)
	if !errors.Is(err, apperr.OutOfStock) || apperr.StatusOf(err) != 409 || apperr.Resolve(err).Message != "库存不足" {
		t.Fatalf("OutOfStock not restored: %v (status %d)", err, apperr.StatusOf(err))
	}
	if err := c.Get(context.Background(), "/notfound", nil); !errors.Is(err, apperr.CouponNotFound) || apperr.StatusOf(err) != 404 {
		t.Fatalf("CouponNotFound not restored: %v", err)
	}
	if err := c.Get(context.Background(), "/ok-status-bad-code", nil); apperr.CodeOf(err) != apperr.CodeSysError {
		t.Fatalf("a non-200 envelope code on HTTP 200 must still be an error, got %v", err)
	}
}

func TestClientDoesNotRetryBusinessErrors(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		biz(w, 409, apperr.CodeOutOfStock, "库存不足")
	}))
	_ = c.Get(context.Background(), "/x", nil)
	if n.Load() != 1 {
		t.Fatalf("4xx business errors must not be retried, server saw %d calls", n.Load())
	}
}

func TestClientRetriesGetOnTransientServerErrorsThenSucceeds(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			biz(w, http.StatusServiceUnavailable, apperr.CodeSystemBusy, "忙")
			return
		}
		ok(w, `{"bucket":1}`)
	}))
	var out deductResp
	if err := c.Get(context.Background(), "/x", &out); err != nil || out.Bucket != 1 {
		t.Fatalf("GET must retry 503 and succeed: %v %+v", err, out)
	}
	if n.Load() != 3 {
		t.Fatalf("server saw %d calls, want 3", n.Load())
	}
}

func TestClientRetriesAreBounded(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); biz(w, 503, apperr.CodeSystemBusy, "忙") }))
	err := c.Get(context.Background(), "/x", nil)
	if n.Load() != 3 { // 默认 2 次重试 = 共 3 次
		t.Fatalf("default retries: server saw %d calls, want 3", n.Load())
	}
	if apperr.StatusOf(err) != 503 || !errors.Is(err, apperr.SystemBusy) {
		t.Fatalf("exhausted retries must surface the upstream error, got %v", err)
	}
	n.Store(0)
	_ = c.Get(context.Background(), "/x", nil, httpx.Retries(0))
	if n.Load() != 1 {
		t.Fatalf("Retries(0) must disable retries, saw %d", n.Load())
	}
	n.Store(0)
	_ = c.Get(context.Background(), "/x", nil, httpx.Retries(4))
	if n.Load() != 5 {
		t.Fatalf("Retries(4) → 5 calls, saw %d", n.Load())
	}
}

func TestClientNeverRetriesNonIdempotentPostUnlessOptedIn(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); biz(w, 503, apperr.CodeSystemBusy, "忙") }))
	_ = c.Post(context.Background(), "/x", deductReq{"r", "1"}, nil)
	if n.Load() != 1 {
		t.Fatalf("POST must not be retried by default (double side effects), saw %d", n.Load())
	}
	n.Store(0)
	_ = c.Post(context.Background(), "/x", deductReq{"r", "1"}, nil, httpx.Idempotent())
	if n.Load() != 3 {
		t.Fatalf("idempotent POST must be retried, saw %d", n.Load())
	}
}

func TestClientReplaysBodyOnRetry(t *testing.T) {
	var bodies []string
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if n.Add(1) == 1 {
			biz(w, 502, apperr.CodeSysError, "bad gateway")
			return
		}
		ok(w, `null`)
	}))
	if err := c.Post(context.Background(), "/x", deductReq{"r9", "7"}, nil, httpx.Idempotent()); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] || !strings.Contains(bodies[1], "r9") {
		t.Fatalf("retry must resend the full body: %q", bodies)
	}
}

func TestClientDoesNotRetryPlain500(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		biz(w, 500, apperr.CodeSysError, "系统错误")
	}))
	err := c.Get(context.Background(), "/x", nil)
	if n.Load() != 1 || apperr.StatusOf(err) != 500 {
		t.Fatalf("a bare 500 is not transient: calls=%d err=%v", n.Load(), err)
	}
}

func TestClientTimeoutBecomesSystemBusyAndRetriesIdempotentCalls(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}), func(o *httpx.ClientOptions) { o.Timeout = 30 * time.Millisecond })
	start := time.Now()
	err := c.Get(context.Background(), "/slow", nil)
	if !errors.Is(err, apperr.SystemBusy) || apperr.StatusOf(err) != 503 {
		t.Fatalf("timeout must map to SystemBusy 503, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the cause must be preserved for logs: %v", err)
	}
	if n.Load() != 3 {
		t.Fatalf("a GET timeout is retried (3 attempts), saw %d", n.Load())
	}
	if time.Since(start) > time.Second {
		t.Fatal("per-attempt timeout not honoured")
	}
	n.Store(0)
	_ = c.Post(context.Background(), "/slow", nil, nil)
	if n.Load() != 1 {
		t.Fatalf("a POST timeout must not be retried, saw %d", n.Load())
	}
}

func TestClientCallTimeoutOverride(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		ok(w, `null`)
	}), func(o *httpx.ClientOptions) { o.Timeout = 5 * time.Millisecond; o.Retries = -1 })
	if err := c.Get(context.Background(), "/x", nil); err == nil {
		t.Fatal("client default timeout should trip")
	}
	if err := c.Get(context.Background(), "/x", nil, httpx.CallTimeout(time.Second)); err != nil {
		t.Fatalf("CallTimeout must override the default: %v", err)
	}
}

func TestClientStopsRetryingWhenCallerContextEnds(t *testing.T) {
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); biz(w, 503, apperr.CodeSystemBusy, "忙") }),
		func(o *httpx.ClientOptions) { o.BackoffBase, o.BackoffMax, o.Retries = time.Hour, time.Hour, 5 })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Get(ctx, "/x", nil) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if apperr.StatusOf(err) != 503 {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation during backoff must return promptly")
	}
	if n.Load() != 1 {
		t.Fatalf("no further attempts after cancel, saw %d", n.Load())
	}
}

func TestClientCallerCancellationIsNotRetriedOrCountedAsUpstreamFailure(t *testing.T) {
	br := limiter.NewBreaker(limiter.BreakerConfig{FailureThreshold: 1})
	var n atomic.Int32
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		<-r.Context().Done()
	}), func(o *httpx.ClientOptions) { o.Breaker = br })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := c.Get(ctx, "/x", nil)
	if apperr.StatusOf(err) != 503 {
		t.Fatalf("got %v", err)
	}
	if n.Load() != 1 || br.State() != limiter.Closed {
		t.Fatalf("caller giving up must not be retried (%d calls) nor trip the breaker (%v)", n.Load(), br.State())
	}
}

func TestClientTransportErrorWhenServerIsDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := httpx.NewClient(httpx.ClientOptions{BaseURL: url, Timeout: time.Second, BackoffBase: time.Millisecond, BackoffMax: time.Millisecond})
	err := c.Get(context.Background(), "/x", nil)
	if !errors.Is(err, apperr.SystemBusy) {
		t.Fatalf("connection refused must map to SystemBusy, got %v", err)
	}
}

func TestClientBreakerOpensAndShortCircuits(t *testing.T) {
	var n atomic.Int32
	br := limiter.NewBreaker(limiter.BreakerConfig{FailureThreshold: 2, OpenTimeout: time.Hour})
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); biz(w, 500, apperr.CodeSysError, "boom") }),
		func(o *httpx.ClientOptions) { o.Breaker = br })
	for i := 0; i < 2; i++ {
		_ = c.Post(context.Background(), "/x", nil, nil)
	}
	if br.State() != limiter.Open {
		t.Fatalf("two upstream failures must open the breaker, state %v", br.State())
	}
	err := c.Post(context.Background(), "/x", nil, nil)
	if !errors.Is(err, apperr.SystemBusy) || !errors.Is(err, limiter.ErrOpen) {
		t.Fatalf("open breaker must fail fast with SystemBusy/ErrOpen, got %v", err)
	}
	if n.Load() != 2 {
		t.Fatalf("no request may reach the upstream while open, saw %d", n.Load())
	}
}

func TestClientBusinessRejectionsDoNotTripBreaker(t *testing.T) {
	br := limiter.NewBreaker(limiter.BreakerConfig{FailureThreshold: 2, OpenTimeout: time.Hour})
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { biz(w, 409, apperr.CodeOutOfStock, "库存不足") }),
		func(o *httpx.ClientOptions) { o.Breaker = br })
	for i := 0; i < 10; i++ {
		_ = c.Post(context.Background(), "/x", nil, nil)
	}
	if br.State() != limiter.Closed {
		t.Fatalf("a healthy upstream saying 'sold out' must not open the breaker, state %v", br.State())
	}
}

func TestClientHandlesGarbageResponsesWithoutLeaking(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html-500":
			w.WriteHeader(500)
			_, _ = w.Write([]byte("<html>stack trace: secret-internal-host</html>"))
		case "/html-200":
			_, _ = w.Write([]byte("<html>login</html>"))
		case "/html-404":
			w.WriteHeader(404)
			_, _ = w.Write([]byte("not here"))
		case "/bad-data":
			ok(w, `{"bucket":"not-a-number"}`)
		}
	}), func(o *httpx.ClientOptions) { o.Retries = -1 })

	err := c.Get(context.Background(), "/html-500", nil)
	if apperr.StatusOf(err) != 500 || strings.Contains(apperr.Resolve(err).Message, "secret-internal-host") {
		t.Fatalf("non-envelope 500 must become a generic 500 error: %v", err)
	}
	if err := c.Get(context.Background(), "/html-200", nil); apperr.StatusOf(err) != 503 {
		t.Fatalf("unparsable success must be SystemBusy, got %v", err)
	}
	if err := c.Get(context.Background(), "/html-404", nil); apperr.StatusOf(err) != 404 {
		t.Fatalf("non-envelope 404 must keep its status, got %v", err)
	}
	var out deductResp
	if err := c.Get(context.Background(), "/bad-data", &out); apperr.StatusOf(err) != 503 {
		t.Fatalf("a data shape mismatch must be an error, got %v", err)
	}
}

func TestClientIgnoresDataWhenOutIsNilAndTrimsBaseURLSlash(t *testing.T) {
	var path string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { path = r.URL.Path; ok(w, `{"anything":1}`) }),
		func(o *httpx.ClientOptions) { o.BaseURL += "/" })
	if err := c.Delete(context.Background(), "/things/1", nil); err != nil || path != "/things/1" {
		t.Fatalf("err=%v path=%q", err, path)
	}
	if err := c.Put(context.Background(), "/things/1", map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
}
