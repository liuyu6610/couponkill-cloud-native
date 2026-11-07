package internalapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// countingServer 返回固定响应并统计调用次数。
func countingServer(status int, body []byte) (http.Handler, *atomic.Int32) {
	var calls atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, status, body)
	}), &calls
}

func TestBusinessErrorsKeepCodeAndStatusAndAreNeverRetried(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       []byte
		wantCode   int
		wantStatus int
		wantIs     *apperr.Error
	}{
		{"409 out of stock", 409, errEnvelope(apperr.CodeOutOfStock, "库存不足"), apperr.CodeOutOfStock, 409, apperr.OutOfStock},
		{"404 coupon not found", 404, errEnvelope(apperr.CodeCouponNotFound, "优惠券不存在"), apperr.CodeCouponNotFound, 404, apperr.CouponNotFound},
		{"409 coupon disabled", 409, errEnvelope(apperr.CodeCouponDisabled, "停用"), apperr.CodeCouponDisabled, 409, apperr.CouponDisabled},
		{"400 invalid request", 400, errEnvelope(apperr.CodeInvalidRequest, "参数错误"), apperr.CodeInvalidRequest, 400, apperr.InvalidRequest},
		{"429 rate limited is a 4xx and not retried", 429, errEnvelope(apperr.CodeRateLimited, "限流"), apperr.CodeRateLimited, 429, apperr.RateLimited},
		{"unknown 4xx code is preserved", 418, errEnvelope(12345, "teapot"), 12345, 418, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, calls := countingServer(tc.status, tc.body)
			c := newCoupon(t, h)
			_, err := c.DeductStock(context.Background(), deductReq)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := apperr.CodeOf(err); got != tc.wantCode {
				t.Errorf("code = %d, want %d", got, tc.wantCode)
			}
			if got := apperr.StatusOf(err); got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(err, %v) = false", tc.wantIs)
			}
			if internalapi.IsTransient(err) {
				t.Error("business errors must not be transient")
			}
			if n := calls.Load(); n != 1 {
				t.Errorf("server calls = %d, want exactly 1 (4xx is never retried)", n)
			}
		})
	}
}

func TestOutOfStockMessageFromServerIsPreserved(t *testing.T) {
	h, _ := countingServer(409, errEnvelope(apperr.CodeOutOfStock, "券已抢光"))
	_, err := newCoupon(t, h).DeductStock(context.Background(), deductReq)
	app, ok := apperr.As(err)
	if !ok || app.Message != "券已抢光" {
		t.Fatalf("app error = %+v ok=%v", app, ok)
	}
}

// 内部调用收到「终端用户鉴权」类错误说明服务间令牌配置有误：
// 绝不能原样透传给终端用户（前端会把用户登出），必须降级为 SysError。
func TestAuthRejectionsOfInternalCallsAreNotSurfacedAsUserAuthErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   []byte
	}{
		{"401 AuthFail envelope", 401, errEnvelope(apperr.CodeAuthFail, "未登录")},
		{"403 Forbidden envelope", 403, errEnvelope(apperr.CodeForbidden, "无权限")},
		{"401 TokenInvalid envelope", 401, errEnvelope(apperr.CodeTokenInvalid, "令牌无效")},
		{"403 raw body", 403, []byte("forbidden")},
		{"404 raw body (missing route)", 404, []byte("404 page not found")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, calls := countingServer(tc.status, tc.body)
			_, err := newCoupon(t, h).DeductStock(context.Background(), deductReq)
			if apperr.CodeOf(err) != apperr.CodeSysError || apperr.StatusOf(err) != 500 {
				t.Fatalf("code=%d status=%d, want SysError/500 (err=%v)", apperr.CodeOf(err), apperr.StatusOf(err), err)
			}
			if internalapi.IsTransient(err) || calls.Load() != 1 {
				t.Fatalf("must be final and not retried (transient=%v calls=%d)", internalapi.IsTransient(err), calls.Load())
			}
		})
	}
}

func TestServerErrorsAreRetriedThenTransient(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       []byte
		wantCode   int
		wantStatus int
	}{
		{"503 busy envelope", 503, errEnvelope(apperr.CodeSystemBusy, "忙"), apperr.CodeSystemBusy, 503},
		{"503 coupon unavailable envelope", 503, errEnvelope(apperr.CodeCouponServiceUnavailable, "不可用"), apperr.CodeCouponServiceUnavailable, 503},
		{"500 sys error envelope", 500, errEnvelope(apperr.CodeSysError, "boom"), apperr.CodeSysError, 500},
		{"502 html from a proxy", 502, []byte("<html>Bad Gateway</html>"), apperr.CodeCouponServiceUnavailable, 503},
		{"504 empty body", 504, nil, apperr.CodeCouponServiceUnavailable, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, calls := countingServer(tc.status, tc.body)
			_, err := newCoupon(t, h).DeductStock(context.Background(), deductReq)
			if !errors.Is(err, internalapi.ErrTransient) || !internalapi.IsTransient(err) {
				t.Fatalf("err = %v, want ErrTransient", err)
			}
			if !errors.Is(err, apperr.SystemBusy) {
				t.Error("ErrTransient must wrap apperr.SystemBusy")
			}
			if got := apperr.CodeOf(err); got != tc.wantCode {
				t.Errorf("code = %d, want %d", got, tc.wantCode)
			}
			if got := apperr.StatusOf(err); got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
			if n := calls.Load(); n != int32(fastRetry.MaxAttempts) {
				t.Errorf("server calls = %d, want %d (bounded retry)", n, fastRetry.MaxAttempts)
			}
		})
	}
}

func TestOrderAndUserClientsMapUnknown5xxToSystemBusy(t *testing.T) {
	h, _ := countingServer(502, []byte("bad gateway"))
	srv := httptest.NewServer(h)
	defer srv.Close()
	oc, err := internalapi.NewOrderClient(srv.URL, testToken, srv.Client(), internalapi.WithRetryPolicy(fastRetry))
	if err != nil {
		t.Fatal(err)
	}
	_, err = oc.GrantOrder(context.Background(), internalapi.GrantRequest{RequestID: "r", UserID: 1, CouponID: 2})
	if apperr.CodeOf(err) != apperr.CodeSystemBusy || !internalapi.IsTransient(err) {
		t.Fatalf("order err = %v", err)
	}
	uc, err := internalapi.NewUserClient(srv.URL, testToken, srv.Client(), internalapi.WithRetryPolicy(fastRetry))
	if err != nil {
		t.Fatal(err)
	}
	_, err = uc.GetUser(context.Background(), 7)
	if apperr.CodeOf(err) != apperr.CodeSystemBusy || !internalapi.IsTransient(err) {
		t.Fatalf("user err = %v", err)
	}
}

func TestSecondAttemptSucceedsAfterTransientFailure(t *testing.T) {
	var calls atomic.Int32
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 503, errEnvelope(apperr.CodeSystemBusy, "忙"))
			return
		}
		writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: 1001, Bucket: 2}))
	}))
	resp, err := c.DeductStock(context.Background(), deductReq)
	if err != nil {
		t.Fatalf("DeductStock: %v", err)
	}
	if resp.Bucket != 2 || calls.Load() != 2 {
		t.Fatalf("resp=%+v calls=%d, want bucket 2 after exactly 2 calls", resp, calls.Load())
	}
}

// ledgerServer 模拟 coupon 的幂等账本：第一次调用真实扣减，但可选地「丢响应」（直接断开连接），
// 重放时按 requestId 返回 alreadyDeducted，不再产生第二次副作用。
type ledgerServer struct {
	mu          sync.Mutex
	deducted    map[string]int // requestId -> bucket
	effects     int            // 真实扣减次数
	calls       int
	dropFirst   bool
	dropCurrent bool
}

func (s *ledgerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req internalapi.DeductRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSON(w, 400, errEnvelope(apperr.CodeInvalidRequest, err.Error()))
		return
	}
	s.mu.Lock()
	s.calls++
	_, already := s.deducted[req.RequestID]
	if !already {
		s.deducted[req.RequestID] = 3
		s.effects++
	}
	drop := s.dropFirst && s.calls == 1
	bucket := s.deducted[req.RequestID]
	s.mu.Unlock()

	if drop { // 响应丢失：副作用已落地，但客户端只看到连接被断开
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
				return
			}
		}
	}
	writeJSON(w, 200, okEnvelope(internalapi.DeductResponse{CouponID: req.CouponID, Bucket: bucket, AlreadyDeducted: already}))
}

func TestIdempotentRetryAfterLostResponseHasNoDoubleEffect(t *testing.T) {
	s := &ledgerServer{deducted: map[string]int{}, dropFirst: true}
	c := newCoupon(t, s)
	resp, err := c.DeductStock(context.Background(), deductReq)
	if err != nil {
		t.Fatalf("DeductStock: %v", err)
	}
	if !resp.AlreadyDeducted || resp.Bucket != 3 {
		t.Fatalf("resp = %+v, want replay of the first deduction", resp)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls != 2 || s.effects != 1 {
		t.Fatalf("calls=%d effects=%d, want 2 calls with exactly 1 effect", s.calls, s.effects)
	}
}

func TestSlowServerTimesOutThenTransient(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body) // 读完请求体后 net/http 才会监听连接断开
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	hc := &http.Client{Timeout: 40 * time.Millisecond}
	c, err := internalapi.NewCouponClient(srv.URL, testToken, hc, internalapi.WithRetryPolicy(fastRetry))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.DeductStock(context.Background(), deductReq)
	if !internalapi.IsTransient(err) || !errors.Is(err, apperr.SystemBusy) {
		t.Fatalf("err = %v, want transient SystemBusy", err)
	}
	if apperr.StatusOf(err) != 503 {
		t.Errorf("status = %d, want 503", apperr.StatusOf(err))
	}
	if n := calls.Load(); n != int32(fastRetry.MaxAttempts) {
		t.Errorf("calls = %d, want %d: timeouts of idempotent calls are retried", n, fastRetry.MaxAttempts)
	}
}

func TestConnectionRefusedIsTransientSystemBusy(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // 地址有效但无人监听
	c, err := internalapi.NewCouponClient(url, testToken, nil, internalapi.WithRetryPolicy(fastRetry))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.DeductStock(context.Background(), deductReq)
	if !internalapi.IsTransient(err) || apperr.CodeOf(err) != apperr.CodeSystemBusy {
		t.Fatalf("err = %v", err)
	}
}

func TestNonIdempotentSyncIsNotRetried(t *testing.T) {
	h, calls := countingServer(503, errEnvelope(apperr.CodeSystemBusy, "忙"))
	c := newCoupon(t, h)
	_, err := c.SyncStock(context.Background(), internalapi.SyncRequest{CouponID: 1001, Available: 10, Source: "connector"})
	if !internalapi.IsTransient(err) {
		t.Fatalf("err = %v, want transient", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1: stock/sync must not be retried", calls.Load())
	}
}

func TestMaxAttemptsOneDisablesRetry(t *testing.T) {
	h, calls := countingServer(503, errEnvelope(apperr.CodeSystemBusy, "忙"))
	c := newCoupon(t, h, internalapi.WithRetryPolicy(internalapi.RetryPolicy{MaxAttempts: 1}))
	if _, err := c.DeductStock(context.Background(), deductReq); !internalapi.IsTransient(err) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestContextCancellationStopsRetries(t *testing.T) {
	var calls atomic.Int32
	first := make(chan struct{}, 1)
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case first <- struct{}{}:
		default:
		}
		writeJSON(w, 503, errEnvelope(apperr.CodeSystemBusy, "忙"))
	}), internalapi.WithRetryPolicy(internalapi.RetryPolicy{MaxAttempts: 10, BaseDelay: 400 * time.Millisecond, MaxDelay: time.Second}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-first
		time.Sleep(20 * time.Millisecond) // 让客户端进入退避等待
		cancel()
	}()

	start := time.Now()
	_, err := c.DeductStock(ctx, deductReq)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if internalapi.IsTransient(err) {
		t.Error("a cancelled call must not be reported as transient (nobody should retry it)")
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("took %v: cancellation must interrupt the backoff sleep", elapsed)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestAlreadyCancelledContextMakesNoCall(t *testing.T) {
	h, calls := countingServer(200, okEnvelope(internalapi.DeductResponse{CouponID: 1001}))
	c := newCoupon(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.DeductStock(ctx, deductReq)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0", calls.Load())
	}
}

func TestContextDeadlineIsNotTransient(t *testing.T) {
	c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // 读完请求体后 net/http 才会监听连接断开
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.DeductStock(ctx, deductReq)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if internalapi.IsTransient(err) {
		t.Error("caller deadline must not be reported as transient")
	}
}

func TestMalformedSuccessResponsesAreFinalSysErrors(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"not json", []byte("<html>ok</html>")},
		{"empty body", nil},
		{"success without data", []byte(`{"code":200,"message":"ok","data":null}`)},
		{"data of the wrong shape", []byte(`{"code":200,"message":"ok","data":{"couponId":"abc"}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, calls := countingServer(200, tc.body)
			_, err := newCoupon(t, h).DeductStock(context.Background(), deductReq)
			if apperr.CodeOf(err) != apperr.CodeSysError || internalapi.IsTransient(err) {
				t.Fatalf("err = %v, want final SysError", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestBusinessCodeInsideHTTP200UsesCanonicalStatus(t *testing.T) {
	h, calls := countingServer(200, errEnvelope(apperr.CodeOutOfStock, "库存不足"))
	_, err := newCoupon(t, h).DeductStock(context.Background(), deductReq)
	if !errors.Is(err, apperr.OutOfStock) || apperr.StatusOf(err) != 409 {
		t.Fatalf("err = %v status=%d, want OutOfStock/409", err, apperr.StatusOf(err))
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestOtherEndpointsSuccessAndErrorMapping(t *testing.T) {
	t.Run("release", func(t *testing.T) {
		var path string
		c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			writeJSON(w, 200, okEnvelope(internalapi.ReleaseResponse{CouponID: 1001, Released: true}))
		}))
		resp, err := c.ReleaseStock(context.Background(), internalapi.ReleaseRequest{RequestID: "r1", CouponID: 1001})
		if err != nil || !resp.Released || path != internalapi.PathStockRelease {
			t.Fatalf("resp=%+v err=%v path=%s", resp, err, path)
		}
	})
	t.Run("sync", func(t *testing.T) {
		var path string
		c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			writeJSON(w, 200, okEnvelope(internalapi.SyncResponse{CouponID: 1001, Available: 77}))
		}))
		resp, err := c.SyncStock(context.Background(), internalapi.SyncRequest{CouponID: 1001, Available: 77, Source: "connector"})
		if err != nil || resp.Available != 77 || path != internalapi.PathStockSync {
			t.Fatalf("resp=%+v err=%v path=%s", resp, err, path)
		}
	})
	t.Run("get coupon", func(t *testing.T) {
		var method, path string
		c := newCoupon(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, path = r.Method, r.URL.Path
			writeJSON(w, 200, okEnvelope(internalapi.CouponInfo{ID: 1001, Name: "券", Type: 2, Status: 1}))
		}))
		resp, err := c.GetCoupon(context.Background(), 1001)
		if err != nil || resp.ID != 1001 || !resp.IsSeckill() || method != http.MethodGet || path != "/internal/v1/coupons/1001" {
			t.Fatalf("resp=%+v err=%v %s %s", resp, err, method, path)
		}
	})
	t.Run("get coupon not found", func(t *testing.T) {
		h, calls := countingServer(404, errEnvelope(apperr.CodeCouponNotFound, "不存在"))
		_, err := newCoupon(t, h).GetCoupon(context.Background(), 9)
		if !errors.Is(err, apperr.CouponNotFound) || calls.Load() != 1 {
			t.Fatalf("err=%v calls=%d", err, calls.Load())
		}
	})
	t.Run("get coupon rejects non-positive id", func(t *testing.T) {
		h, calls := countingServer(200, nil)
		_, err := newCoupon(t, h).GetCoupon(context.Background(), 0)
		if !errors.Is(err, apperr.InvalidRequest) || calls.Load() != 0 {
			t.Fatalf("err=%v calls=%d", err, calls.Load())
		}
	})
	t.Run("grant", func(t *testing.T) {
		var gotToken, path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotToken, path = r.Header.Get(internalapi.HeaderInternalToken), r.URL.Path
			writeJSON(w, 200, okEnvelope(internalapi.GrantResponse{OrderID: 5551234567890123456}))
		}))
		defer srv.Close()
		oc, _ := internalapi.NewOrderClient(srv.URL, testToken, srv.Client())
		resp, err := oc.GrantOrder(context.Background(), internalapi.GrantRequest{RequestID: "r", UserID: 1, CouponID: 2})
		if err != nil || resp.OrderID != 5551234567890123456 || gotToken != testToken || path != internalapi.PathOrderGrant {
			t.Fatalf("resp=%+v err=%v token=%q path=%s", resp, err, gotToken, path)
		}
	})
	t.Run("grant repeat stays RepeatSeckill and is not retried", func(t *testing.T) {
		h, calls := countingServer(409, errEnvelope(apperr.CodeRepeatSeckill, "重复"))
		srv := httptest.NewServer(h)
		defer srv.Close()
		oc, _ := internalapi.NewOrderClient(srv.URL, testToken, srv.Client(), internalapi.WithRetryPolicy(fastRetry))
		_, err := oc.GrantOrder(context.Background(), internalapi.GrantRequest{RequestID: "r", UserID: 1, CouponID: 2})
		if !errors.Is(err, apperr.RepeatSeckill) || calls.Load() != 1 {
			t.Fatalf("err=%v calls=%d", err, calls.Load())
		}
	})
	t.Run("get user", func(t *testing.T) {
		var path string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			writeJSON(w, 200, okEnvelope(internalapi.UserInfo{ID: 7, Username: "demo", Role: "user", Status: 1}))
		}))
		defer srv.Close()
		uc, _ := internalapi.NewUserClient(srv.URL, testToken, srv.Client())
		u, err := uc.GetUser(context.Background(), 7)
		if err != nil || u.Username != "demo" || path != "/internal/v1/users/7" {
			t.Fatalf("user=%+v err=%v path=%s", u, err, path)
		}
	})
	t.Run("get user not found", func(t *testing.T) {
		h, _ := countingServer(404, errEnvelope(apperr.CodeUserNotFound, "无此用户"))
		srv := httptest.NewServer(h)
		defer srv.Close()
		uc, _ := internalapi.NewUserClient(srv.URL, testToken, srv.Client())
		_, err := uc.GetUser(context.Background(), 7)
		if !errors.Is(err, apperr.UserNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
}

// 并发下同一 requestId 的重复扣减不得产生第二次副作用（服务端账本语义 + 客户端重试共同保证）。
func TestConcurrentDuplicateDeductsProduceOneEffect(t *testing.T) {
	s := &ledgerServer{deducted: map[string]int{}}
	c := newCoupon(t, s)
	const workers = 16
	var wg sync.WaitGroup
	var already atomic.Int32
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.DeductStock(context.Background(), deductReq)
			if err != nil {
				t.Errorf("DeductStock: %v", err)
				return
			}
			if resp.AlreadyDeducted {
				already.Add(1)
			}
		}()
	}
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.effects != 1 {
		t.Fatalf("effects = %d, want 1", s.effects)
	}
	if already.Load() != workers-1 {
		t.Fatalf("alreadyDeducted replies = %d, want %d", already.Load(), workers-1)
	}
}
