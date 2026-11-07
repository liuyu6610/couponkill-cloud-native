package internalapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

const testToken = "internal-secret-token"

// fastRetry 让重试测试在毫秒内完成。
var fastRetry = internalapi.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 4 * time.Millisecond}

func okEnvelope(data any) []byte {
	raw, _ := json.Marshal(data)
	out, _ := json.Marshal(map[string]any{"code": 200, "message": "ok", "data": json.RawMessage(raw)})
	return out
}

func errEnvelope(code int, msg string) []byte {
	out, _ := json.Marshal(map[string]any{"code": code, "message": msg, "data": nil})
	return out
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func newCoupon(t *testing.T, h http.Handler, opts ...internalapi.Option) *internalapi.CouponClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts = append([]internalapi.Option{internalapi.WithRetryPolicy(fastRetry)}, opts...)
	c, err := internalapi.NewCouponClient(srv.URL, testToken, srv.Client(), opts...)
	if err != nil {
		t.Fatalf("NewCouponClient: %v", err)
	}
	return c
}

var deductReq = internalapi.DeductRequest{RequestID: "req-1", CouponID: 1001, UserID: 42}

func TestDeductStockDecodesEnvelopeAndSendsHeaders(t *testing.T) {
	var gotMethod, gotPath, gotToken, gotReqID, gotCT string
	var gotBody internalapi.DeductRequest
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotToken = r.Header.Get(internalapi.HeaderInternalToken)
		gotReqID = r.Header.Get(internalapi.HeaderRequestID)
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: 1001, Bucket: 5, AlreadyDeducted: false}))
	}))

	ctx := internalapi.WithRequestID(context.Background(), "trace-xyz")
	resp, err := c.DeductStock(ctx, deductReq)
	if err != nil {
		t.Fatalf("DeductStock: %v", err)
	}
	if resp.CouponID != 1001 || resp.Bucket != 5 || resp.AlreadyDeducted {
		t.Fatalf("response = %+v", resp)
	}
	if gotMethod != http.MethodPost || gotPath != internalapi.PathStockDeduct {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotToken != testToken {
		t.Errorf("token header = %q", gotToken)
	}
	if gotReqID != "trace-xyz" {
		t.Errorf("request id header = %q, want propagated from ctx", gotReqID)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotBody != deductReq {
		t.Errorf("body = %+v, want %+v", gotBody, deductReq)
	}
}

func TestRequestIDHeaderOmittedWhenContextHasNone(t *testing.T) {
	var present atomic.Bool
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Header[http.CanonicalHeaderKey(internalapi.HeaderRequestID)]
		present.Store(ok)
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: 1001}))
	}))
	if _, err := c.DeductStock(context.Background(), deductReq); err != nil {
		t.Fatal(err)
	}
	if present.Load() {
		t.Fatal("X-Request-Id must be absent when the context carries none")
	}
}

func TestRequestIDSourceOptionIsUsed(t *testing.T) {
	var got string
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(internalapi.HeaderRequestID)
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: 1001}))
	}), internalapi.WithRequestIDSource(func(context.Context) string { return "from-logx" }))
	if _, err := c.DeductStock(context.Background(), deductReq); err != nil {
		t.Fatal(err)
	}
	if got != "from-logx" {
		t.Fatalf("request id = %q, want from-logx", got)
	}
}

func TestInvalidRequestFailsFastWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{}))
	}))
	_, err := c.DeductStock(context.Background(), internalapi.DeductRequest{})
	if !errors.Is(err, apperr.InvalidRequest) {
		t.Fatalf("err = %v, want InvalidRequest", err)
	}
	if calls.Load() != 0 {
		t.Fatal("invalid requests must not hit the network")
	}
}

func TestNewClientsRejectBadBaseURL(t *testing.T) {
	for _, base := range []string{"", "not a url", "ftp://host", "http://", "/relative"} {
		if _, err := internalapi.NewCouponClient(base, testToken, nil); err == nil {
			t.Errorf("NewCouponClient(%q) should fail", base)
		}
		if _, err := internalapi.NewOrderClient(base, testToken, nil); err == nil {
			t.Errorf("NewOrderClient(%q) should fail", base)
		}
		if _, err := internalapi.NewUserClient(base, testToken, nil); err == nil {
			t.Errorf("NewUserClient(%q) should fail", base)
		}
	}
	if _, err := internalapi.NewCouponClient("http://coupon:8080", "", nil); err == nil {
		t.Error("empty token must be rejected")
	}
}

func TestBaseURLWithTrailingSlashAndNilHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != internalapi.PathStockDeduct {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: 1001}))
	}))
	defer srv.Close()
	c, err := internalapi.NewCouponClient(srv.URL+"/", testToken, nil) // nil -> 默认带超时的 client
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeductStock(context.Background(), deductReq); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultHTTPClientHasTimeouts(t *testing.T) {
	hc := internalapi.DefaultHTTPClient()
	if hc.Timeout <= 0 {
		t.Fatal("default client must have an overall timeout")
	}
}
