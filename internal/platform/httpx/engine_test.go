package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s.String()), "\n") {
		if line == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line not JSON: %q", line)
		}
		out = append(out, m)
	}
	return out
}

func newEngine(t *testing.T, mut ...func(*httpx.Options)) (*httpx.Engine, *syncBuf) {
	t.Helper()
	buf := &syncBuf{}
	opts := httpx.Options{Service: "svc", Logger: logx.New(logx.Options{Level: "debug", Writer: buf})}
	for _, m := range mut {
		m(&opts)
	}
	return httpx.NewEngine(opts), buf
}

// ---------- 信封 ----------

type item struct {
	ID idgen.ID `json:"id"`
}

func TestEnvelopeHelpers(t *testing.T) {
	e, logs := newEngine(t)
	e.GET("/ok", func(c *gin.Context) { httpx.OK(c, gin.H{"a": 1}) })
	e.GET("/nil", func(c *gin.Context) { httpx.OK(c, nil) })
	e.POST("/accepted", func(c *gin.Context) { httpx.Accepted(c, gin.H{"requestId": "r1", "status": "QUEUED"}) })
	e.GET("/page", func(c *gin.Context) { httpx.Page(c, []item{{ID: 9007199254740993}}, 41, 2, 20) })
	e.GET("/page-nil", func(c *gin.Context) { httpx.Page(c, []item(nil), 0, 1, 20) })
	e.GET("/page-untyped-nil", func(c *gin.Context) { httpx.Page(c, nil, 0, 1, 20) })
	e.GET("/biz", func(c *gin.Context) { httpx.Fail(c, apperr.OutOfStock) })
	e.GET("/wrapped", func(c *gin.Context) { httpx.Fail(c, fmt.Errorf("deduct: %w", apperr.CouponNotFound)) })
	e.GET("/plain", func(c *gin.Context) { httpx.Fail(c, errors.New("pq: password authentication failed for user hunter2")) })
	e.GET("/custom", func(c *gin.Context) { httpx.Fail(c, apperr.RateLimited.WithMessage("太快了")) })

	res := testkit.DoJSON(t, e, "GET", "/ok", nil)
	if res.Status != 200 || res.Code != 200 || res.Message != "ok" || res.RequestID != "" {
		t.Fatalf("OK envelope wrong: %+v", res)
	}
	var m map[string]int
	res.Decode(&m)
	if m["a"] != 1 {
		t.Fatalf("data wrong: %v", m)
	}
	if r := testkit.DoJSON(t, e, "GET", "/nil", nil); r.Status != 200 || !r.DataIsNull() {
		t.Fatalf("OK(nil) must give data:null: %s", r.Body)
	}
	if r := testkit.DoJSON(t, e, "POST", "/accepted", nil); r.Status != 202 || r.Code != 200 {
		t.Fatalf("Accepted must be HTTP 202: %+v", r)
	}

	page := testkit.DoJSON(t, e, "GET", "/page", nil)
	if !strings.Contains(string(page.Body), `"items":[{"id":"9007199254740993"}]`) || !strings.Contains(string(page.Body), `"total":41,"page":2,"size":20`) {
		t.Fatalf("page body wrong (IDs must be strings): %s", page.Body)
	}
	for _, p := range []string{"/page-nil", "/page-untyped-nil"} {
		if b := testkit.DoJSON(t, e, "GET", p, nil).Body; !strings.Contains(string(b), `"items":[]`) {
			t.Fatalf("%s: nil items must render as []: %s", p, b)
		}
	}

	for _, tc := range []struct {
		path   string
		status int
		code   int
	}{{"/biz", 409, 10002}, {"/wrapped", 404, 10008}, {"/plain", 500, 50000}, {"/custom", 429, 10003}} {
		r := testkit.DoJSON(t, e, "GET", tc.path, nil)
		if r.Status != tc.status || r.Code != tc.code || !r.DataIsNull() {
			t.Errorf("%s: got status=%d code=%d body=%s, want %d/%d with data:null", tc.path, r.Status, r.Code, r.Body, tc.status, tc.code)
		}
		if r.RequestID == "" || r.RequestID != r.Header.Get("X-Request-Id") {
			t.Errorf("%s: failure body must carry the request id matching the header (%q vs %q)", tc.path, r.RequestID, r.Header.Get("X-Request-Id"))
		}
	}
	plain := testkit.DoJSON(t, e, "GET", "/plain", nil)
	if strings.Contains(string(plain.Body), "hunter2") || plain.Message != "系统错误" {
		t.Fatalf("internal error details must never reach the body: %s", plain.Body)
	}
	if !strings.Contains(logs.String(), "hunter2") {
		t.Fatal("...but the cause must be logged")
	}
	if r := testkit.DoJSON(t, e, "GET", "/custom", nil); r.Message != "太快了" {
		t.Fatalf("custom message lost: %s", r.Body)
	}
}

func TestFailAbortsTheChain(t *testing.T) {
	e, _ := newEngine(t)
	reached := false
	e.GET("/x", func(c *gin.Context) { httpx.Fail(c, apperr.Forbidden) }, func(c *gin.Context) { reached = true })
	if r := testkit.DoJSON(t, e, "GET", "/x", nil); r.Status != 403 || reached {
		t.Fatalf("handlers after Fail must not run (status %d, reached %v)", r.Status, reached)
	}
}

func TestFailWithNilErrorIsASystemError(t *testing.T) {
	e, _ := newEngine(t)
	e.GET("/x", func(c *gin.Context) { httpx.Fail(c, nil) })
	if r := testkit.DoJSON(t, e, "GET", "/x", nil); r.Status != 500 || r.Code != 50000 {
		t.Fatalf("Fail(nil) is a programming error and must surface as 500: %+v", r)
	}
}

func TestNotFoundAndMethodNotAllowedUseEnvelope(t *testing.T) {
	e, _ := newEngine(t)
	e.GET("/only-get", func(c *gin.Context) { httpx.OK(c, nil) })
	nf := testkit.DoJSON(t, e, "GET", "/nope", nil)
	if nf.Status != 404 || nf.Code != apperr.CodeInvalidRequest || nf.Message == "" {
		t.Fatalf("404 envelope wrong: %+v", nf)
	}
	mna := testkit.DoJSON(t, e, "POST", "/only-get", nil)
	if mna.Status != 405 || mna.Code != apperr.CodeInvalidRequest {
		t.Fatalf("405 envelope wrong: %+v", mna)
	}
}

// ---------- request id ----------

func TestRequestID(t *testing.T) {
	e, logs := newEngine(t)
	e.GET("/x", func(c *gin.Context) {
		logx.From(c.Request.Context()).Info("inside handler")
		httpx.OK(c, gin.H{"id": httpx.RequestID(c)})
	})
	hex32 := regexp.MustCompile(`^[0-9a-f]{32}$`)

	gen := testkit.DoJSON(t, e, "GET", "/x", nil)
	if id := gen.Header.Get("X-Request-Id"); !hex32.MatchString(id) {
		t.Fatalf("missing id must be generated, got %q", id)
	}
	kept := testkit.DoJSON(t, e, "GET", "/x", nil, testkit.Header("X-Request-Id", "trace-abc_1.2"))
	if kept.Header.Get("X-Request-Id") != "trace-abc_1.2" {
		t.Fatalf("a valid inbound id must be kept, got %q", kept.Header.Get("X-Request-Id"))
	}
	var data map[string]string
	kept.Decode(&data)
	if data["id"] != "trace-abc_1.2" {
		t.Fatalf("handler must see the same id: %v", data)
	}
	for _, bad := range []string{"has space", "x\"y", strings.Repeat("a", 65), "日本語", "a/b"} {
		r := testkit.DoJSON(t, e, "GET", "/x", nil, testkit.Header("X-Request-Id", bad))
		if id := r.Header.Get("X-Request-Id"); id == bad || !hex32.MatchString(id) {
			t.Errorf("unsafe inbound id %q must be replaced, got %q", bad, id)
		}
	}
	found := false
	for _, rec := range logs.records(t) {
		if rec["msg"] == "inside handler" && rec["request_id"] == "trace-abc_1.2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("handler logs must carry request_id:\n%s", logs.String())
	}
}

// ---------- recover ----------

func TestPanicIsRecoveredIntoAnEnvelope(t *testing.T) {
	e, logs := newEngine(t)
	e.GET("/boom", func(c *gin.Context) { panic("kaboom secret-detail") })
	e.GET("/ok", func(c *gin.Context) { httpx.OK(c, nil) })

	r := testkit.DoJSON(t, e, "GET", "/boom", nil)
	if r.Status != 500 || r.Code != 50000 || strings.Contains(string(r.Body), "kaboom") {
		t.Fatalf("panic must become a clean 500 envelope: %+v", r)
	}
	if !strings.Contains(logs.String(), "kaboom") || !strings.Contains(logs.String(), "stack") {
		t.Fatalf("panic and stack must be logged:\n%s", logs.String())
	}
	if r := testkit.DoJSON(t, e, "GET", "/ok", nil); r.Status != 200 {
		t.Fatal("engine must keep serving after a panic")
	}
}

func TestPanicAfterResponseStartedDoesNotCorruptStatus(t *testing.T) {
	e, _ := newEngine(t)
	e.GET("/late", func(c *gin.Context) {
		c.String(http.StatusOK, "partial")
		c.Writer.Flush()
		panic("late")
	})
	r := testkit.DoJSON(t, e, "GET", "/late", nil)
	if r.Status != 200 || string(r.Body) != "partial" {
		t.Fatalf("already-written responses must be left alone: %d %q", r.Status, r.Body)
	}
}

func TestErrAbortHandlerPropagates(t *testing.T) {
	e, _ := newEngine(t)
	e.GET("/abort", func(c *gin.Context) { panic(http.ErrAbortHandler) })
	defer func() {
		if r := recover(); r != http.ErrAbortHandler {
			t.Fatalf("http.ErrAbortHandler must propagate to net/http, got %v", r)
		}
	}()
	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/abort", nil))
	t.Fatal("expected a panic")
}

// ---------- 访问日志 ----------

func TestAccessLogFields(t *testing.T) {
	e, logs := newEngine(t)
	e.Use(func(c *gin.Context) { c.Next() })
	g := e.Group("/api/v1", httpx.RequireGatewayIdentity(testkit.Signer(t)))
	g.GET("/coupons/:id", func(c *gin.Context) { httpx.OK(c, nil) })
	g.GET("/deny", func(c *gin.Context) { httpx.Fail(c, apperr.OutOfStock) })

	testkit.DoJSON(t, e, "GET", "/api/v1/coupons/1001?token=secret", nil, testkit.User(42, "user"))
	testkit.DoJSON(t, e, "GET", "/api/v1/deny", nil, testkit.User(7))
	testkit.DoJSON(t, e, "GET", "/nope", nil)
	testkit.DoJSON(t, e, "GET", "/api/v1/coupons/1", nil) // 无身份 → 401

	var access []map[string]any
	for _, r := range logs.records(t) {
		if r["msg"] == "http request" {
			access = append(access, r)
		}
	}
	if len(access) != 4 {
		t.Fatalf("want 4 access log lines, got %d:\n%s", len(access), logs.String())
	}
	a := access[0]
	if a["method"] != "GET" || a["path"] != "/api/v1/coupons/1001" || a["route"] != "/api/v1/coupons/:id" ||
		a["status"] != float64(200) || a["user_id"] != float64(42) || a["level"] != "INFO" || a["ip"] != "192.0.2.1" {
		t.Fatalf("access log fields wrong: %v", a)
	}
	if _, ok := a["latency_ms"].(float64); !ok {
		t.Fatalf("latency_ms missing: %v", a)
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatal("the query string must not be logged (it may carry tokens)")
	}
	if access[1]["code"] != float64(10002) || access[1]["status"] != float64(409) || access[1]["level"] != "WARN" {
		t.Fatalf("business rejections must log code at WARN: %v", access[1])
	}
	if access[2]["route"] != "unmatched" || access[2]["status"] != float64(404) {
		t.Fatalf("unmatched route label wrong: %v", access[2])
	}
	if _, has := access[3]["user_id"]; has || access[3]["status"] != float64(401) {
		t.Fatalf("unauthenticated request must not log a user id: %v", access[3])
	}
	// 同一个 key 不得在一行里出现两次（ctx logger 与访问日志属性重复）。
	for _, line := range strings.Split(logs.String(), "\n") {
		if n := strings.Count(line, `"user_id"`); n > 1 {
			t.Fatalf("user_id duplicated within one log line: %s", line)
		}
	}
}

func TestOpsEndpointsLogAtDebugOnly(t *testing.T) {
	buf := &syncBuf{}
	e := httpx.NewEngine(httpx.Options{Service: "svc", Logger: logx.New(logx.Options{Level: "info", Writer: buf})})
	for _, p := range []string{"/healthz", "/readyz", "/metrics"} {
		testkit.DoJSON(t, e, "GET", p, nil)
	}
	if buf.String() != "" {
		t.Fatalf("probe traffic must not flood info logs:\n%s", buf.String())
	}
}

// ---------- 绑定 ----------

type createReq struct {
	CouponID idgen.ID `json:"couponId" binding:"required"`
	Name     string   `json:"name" binding:"required,min=2,max=8"`
	Size     int      `json:"size" binding:"min=1,max=100"`
	Kind     string   `json:"kind" binding:"omitempty,oneof=a b"`
}

func bindEngine(t *testing.T, opts ...func(*httpx.Options)) *httpx.Engine {
	e, _ := newEngine(t, opts...)
	e.POST("/create", func(c *gin.Context) {
		req, ok := httpx.BindJSON[createReq](c)
		if !ok {
			return
		}
		httpx.OK(c, req)
	})
	return e
}

func TestBindJSONFieldLevelErrors(t *testing.T) {
	e := bindEngine(t)
	cases := []struct {
		name string
		body any
		want []string
	}{
		{"missing required", `{"size":5}`, []string{"couponId 为必填项", "name 为必填项"}},
		{"too short", `{"couponId":"1","name":"x","size":5}`, []string{"name 长度不能小于 2"}},
		{"too long", `{"couponId":"1","name":"abcdefghij","size":5}`, []string{"name 长度不能大于 8"}},
		{"number below min", `{"couponId":"1","name":"ab","size":0}`, []string{"size 不能小于 1"}},
		{"number above max", `{"couponId":"1","name":"ab","size":101}`, []string{"size 不能大于 100"}},
		{"oneof", `{"couponId":"1","name":"ab","size":5,"kind":"z"}`, []string{"kind 必须是 [a b] 之一"}},
		{"wrong type", `{"couponId":"1","name":"ab","size":"five"}`, []string{"size 类型错误"}},
		{"bad id string", `{"couponId":"abc","name":"ab","size":5}`, []string{"请求参数错误"}},
		{"invalid json", `{"couponId":`, []string{"请求体不是合法的 JSON"}},
		{"empty body", ``, []string{"请求体不能为空"}},
	}
	for _, tc := range cases {
		r := testkit.DoJSON(t, e, "POST", "/create", tc.body)
		if r.Status != 400 || r.Code != apperr.CodeInvalidRequest || !r.DataIsNull() {
			t.Errorf("%s: want 400/10004 got %d/%d (%s)", tc.name, r.Status, r.Code, r.Body)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(r.Message, w) {
				t.Errorf("%s: message %q must contain %q", tc.name, r.Message, w)
			}
		}
	}
}

func TestBindJSONAcceptsValidInputIncludingStringAndNumericIDs(t *testing.T) {
	e := bindEngine(t)
	for _, body := range []string{
		`{"couponId":"9007199254740993","name":"abc","size":10}`,
		`{"couponId":9007199254740993,"name":"abc","size":10,"extra":"ignored"}`,
	} {
		r := testkit.DoJSON(t, e, "POST", "/create", body)
		if r.Status != 200 {
			t.Fatalf("valid body rejected: %s -> %s", body, r.Body)
		}
		var out createReq
		r.Decode(&out)
		if out.CouponID != 9007199254740993 || out.Name != "abc" {
			t.Fatalf("decoded wrong: %+v", out)
		}
	}
}

func TestBindJSONCapsReportedProblems(t *testing.T) {
	type many struct {
		A int `json:"a" binding:"required"`
		B int `json:"b" binding:"required"`
		C int `json:"c" binding:"required"`
		D int `json:"d" binding:"required"`
		E int `json:"e" binding:"required"`
		F int `json:"f" binding:"required"`
		G int `json:"g" binding:"required"`
	}
	e, _ := newEngine(t)
	e.POST("/x", func(c *gin.Context) {
		if _, ok := httpx.BindJSON[many](c); ok {
			httpx.OK(c, nil)
		}
	})
	r := testkit.DoJSON(t, e, "POST", "/x", `{}`)
	if strings.Count(r.Message, "为必填项") != 5 || !strings.Contains(r.Message, "等 7 项") {
		t.Fatalf("problem list must be capped with a total: %q", r.Message)
	}
}

func TestBindQueryAndURI(t *testing.T) {
	type q struct {
		Page int    `form:"page" binding:"min=1"`
		Size int    `form:"size" binding:"min=1,max=50"`
		Sort string `form:"sort"`
	}
	type u struct {
		ID idgen.ID `uri:"id" binding:"required"`
	}
	e, _ := newEngine(t)
	e.GET("/list", func(c *gin.Context) {
		v, ok := httpx.BindQuery[q](c)
		if ok {
			httpx.OK(c, v)
		}
	})
	e.GET("/things/:id", func(c *gin.Context) {
		v, ok := httpx.BindURI[u](c)
		if ok {
			httpx.OK(c, gin.H{"id": v.ID})
		}
	})
	if r := testkit.DoJSON(t, e, "GET", "/list?page=2&size=10&sort=asc", nil); r.Status != 200 {
		t.Fatalf("valid query rejected: %s", r.Body)
	}
	bad := testkit.DoJSON(t, e, "GET", "/list?page=0&size=99", nil)
	if bad.Status != 400 || !strings.Contains(bad.Message, "page 不能小于 1") || !strings.Contains(bad.Message, "size 不能大于 50") {
		t.Fatalf("query validation message wrong: %s", bad.Body)
	}
	if r := testkit.DoJSON(t, e, "GET", "/list?page=abc&size=1", nil); r.Status != 400 {
		t.Fatalf("unparsable query must be 400, got %d", r.Status)
	}
	ok := testkit.DoJSON(t, e, "GET", "/things/9007199254740993", nil)
	if ok.Status != 200 || !strings.Contains(string(ok.Body), `"id":"9007199254740993"`) {
		t.Fatalf("uri bind wrong: %s", ok.Body)
	}
	if r := testkit.DoJSON(t, e, "GET", "/things/not-a-number", nil); r.Status != 400 {
		t.Fatalf("bad uri id must be 400, got %d", r.Status)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	e := bindEngine(t, func(o *httpx.Options) { o.MaxBodyBytes = 32 })
	big := `{"couponId":"1","name":"abc","size":10,"pad":"` + strings.Repeat("x", 200) + `"}`
	r := testkit.DoJSON(t, e, "POST", "/create", big)
	if r.Status != 400 || !strings.Contains(r.Message, "请求体过大") {
		t.Fatalf("oversized body must be rejected cleanly: %d %s", r.Status, r.Body)
	}
}

// ---------- 健康检查 ----------

func TestHealthzAlwaysAliveReadyzAggregatesChecks(t *testing.T) {
	e, logs := newEngine(t)
	if r := testkit.DoJSON(t, e, "GET", "/healthz", nil); r.Status != 200 {
		t.Fatalf("healthz: %d", r.Status)
	}
	if r := testkit.DoJSON(t, e, "GET", "/readyz", nil); r.Status != 200 {
		t.Fatalf("readyz with no checks must be ready: %d", r.Status)
	}
	redisUp := true
	e.Health.Add("redis", func(context.Context) error {
		if !redisUp {
			return errors.New("connection refused")
		}
		return nil
	})
	e.Health.Add("pg", func(context.Context) error { return nil })

	ok := testkit.DoJSON(t, e, "GET", "/readyz", nil)
	var data struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	ok.Decode(&data)
	if ok.Status != 200 || data.Status != "ready" || data.Checks["redis"] != "ok" || data.Checks["pg"] != "ok" {
		t.Fatalf("ready response wrong: %s", ok.Body)
	}

	redisUp = false
	bad := testkit.DoJSON(t, e, "GET", "/readyz", nil)
	bad.Decode(&data)
	if bad.Status != 503 || bad.Code != apperr.CodeSystemBusy || data.Status != "not_ready" || data.Checks["redis"] != "fail" || data.Checks["pg"] != "ok" {
		t.Fatalf("a failing check must flip readiness: %s", bad.Body)
	}
	if strings.Contains(string(bad.Body), "connection refused") {
		t.Fatal("check error details must not leak to the response")
	}
	if !strings.Contains(logs.String(), "connection refused") {
		t.Fatal("check failures must be logged")
	}
	// 存活探针不受依赖影响。
	if r := testkit.DoJSON(t, e, "GET", "/healthz", nil); r.Status != 200 {
		t.Fatal("liveness must not depend on dependencies")
	}
}

func TestReadyzChecksRunConcurrentlyWithTimeout(t *testing.T) {
	e, _ := newEngine(t)
	e.Health.Add("slow-a", func(ctx context.Context) error { time.Sleep(120 * time.Millisecond); return nil }, time.Second)
	e.Health.Add("slow-b", func(ctx context.Context) error { time.Sleep(120 * time.Millisecond); return nil }, time.Second)
	start := time.Now()
	if r := testkit.DoJSON(t, e, "GET", "/readyz", nil); r.Status != 200 {
		t.Fatalf("status %d", r.Status)
	}
	if d := time.Since(start); d > 230*time.Millisecond {
		t.Fatalf("checks must run in parallel, took %v", d)
	}

	e.Health.Add("hangs", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 30*time.Millisecond)
	start = time.Now()
	r := testkit.DoJSON(t, e, "GET", "/readyz", nil)
	if r.Status != 503 {
		t.Fatalf("a timed-out check must fail readiness, got %d", r.Status)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("per-check timeout not honoured: %v", d)
	}
}

func TestReadyzFlipsOnShutdown(t *testing.T) {
	e, _ := newEngine(t)
	e.Health.Add("pg", func(context.Context) error { return nil })
	if r := testkit.DoJSON(t, e, "GET", "/readyz", nil); r.Status != 200 {
		t.Fatal("must start ready")
	}
	e.Health.SetShuttingDown()
	r := testkit.DoJSON(t, e, "GET", "/readyz", nil)
	if r.Status != 503 || !strings.Contains(string(r.Body), "shutting_down") {
		t.Fatalf("after SetShuttingDown readyz must be 503/shutting_down: %d %s", r.Status, r.Body)
	}
	if testkit.DoJSON(t, e, "GET", "/healthz", nil).Status != 200 {
		t.Fatal("liveness stays up while draining")
	}
}

// ---------- 指标 ----------

func TestMetricsExposeRequestCountsByRouteTemplate(t *testing.T) {
	e, _ := newEngine(t)
	e.GET("/coupons/:id", func(c *gin.Context) { httpx.OK(c, nil) })
	for i := 0; i < 3; i++ {
		testkit.DoJSON(t, e, "GET", fmt.Sprintf("/coupons/%d", i), nil)
	}
	testkit.DoJSON(t, e, "GET", "/scanner/probe/wp-admin", nil)

	body := string(testkit.DoJSON(t, e, "GET", "/metrics", nil).Body)
	want := `ck_http_requests_total{method="GET",route="/coupons/:id",service="svc",status="200"} 3`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics must count by route template; want %q in:\n%s", want, firstLines(body, "ck_http"))
	}
	if strings.Contains(body, "/coupons/1") || strings.Contains(body, "wp-admin") {
		t.Fatal("raw paths must never become label values (cardinality explosion)")
	}
	if !strings.Contains(body, `route="unmatched"`) || !strings.Contains(body, "ck_http_request_duration_seconds_bucket") || !strings.Contains(body, "ck_http_inflight_requests") {
		t.Fatal("duration histogram, inflight gauge and unmatched label expected")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Fatal("go runtime metrics expected")
	}
}

func TestEnginesHaveIndependentRegistries(t *testing.T) {
	a, _ := newEngine(t)
	b, _ := newEngine(t) // 同进程创建第二个引擎不得因重复注册而 panic
	a.GET("/x", func(c *gin.Context) { httpx.OK(c, nil) })
	testkit.DoJSON(t, a, "GET", "/x", nil)
	if strings.Contains(string(testkit.DoJSON(t, b, "GET", "/metrics", nil).Body), `route="/x"`) {
		t.Fatal("registries must not be shared between engines")
	}
}

func firstLines(s, prefix string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// ---------- 鉴权中间件 ----------

func TestRequireInternalToken(t *testing.T) {
	e, _ := newEngine(t)
	e.POST("/internal/v1/x", httpx.RequireInternalToken(testkit.InternalToken), func(c *gin.Context) { httpx.OK(c, nil) })
	e.POST("/internal/v1/unconfigured", httpx.RequireInternalToken(""), func(c *gin.Context) { httpx.OK(c, nil) })

	if r := testkit.DoJSON(t, e, "POST", "/internal/v1/x", nil, testkit.Internal()); r.Status != 200 {
		t.Fatalf("valid token rejected: %s", r.Body)
	}
	for name, hdr := range map[string]string{"missing": "", "wrong": "nope", "prefix": testkit.InternalToken[:10], "longer": testkit.InternalToken + "x"} {
		var opts []testkit.ReqOption
		if hdr != "" {
			opts = append(opts, testkit.Header("X-Internal-Token", hdr))
		}
		if r := testkit.DoJSON(t, e, "POST", "/internal/v1/x", nil, opts...); r.Status != 401 || r.Code != apperr.CodeAuthFail {
			t.Errorf("%s token must give 401/20001, got %d/%d", name, r.Status, r.Code)
		}
	}
	// 漏配密钥（空串）时，空的请求头不能“匹配成功”。
	if r := testkit.DoJSON(t, e, "POST", "/internal/v1/unconfigured", nil); r.Status != 401 {
		t.Fatalf("an unconfigured token must fail closed, got %d", r.Status)
	}
	if r := testkit.DoJSON(t, e, "POST", "/internal/v1/unconfigured", nil, testkit.Header("X-Internal-Token", "")); r.Status != 401 {
		t.Fatalf("empty header vs empty token must still be rejected, got %d", r.Status)
	}
}

func TestRequireGatewayIdentity(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	signer, err := authx.NewGatewaySigner(testkit.GatewayKey, authx.WithSignerClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := newEngine(t)
	g := e.Group("/api/v1", httpx.RequireGatewayIdentity(signer))
	g.GET("/me", func(c *gin.Context) {
		id, ok := httpx.IdentityFrom(c)
		cid, cok := authx.IdentityFrom(c.Request.Context())
		httpx.OK(c, gin.H{"id": id.UserID, "roles": id.Roles, "ok": ok, "ctxOK": cok, "ctxID": cid.UserID})
	})

	r := testkit.DoJSON(t, e, "GET", "/api/v1/me", nil, testkit.User(42, "user", "admin"))
	var me struct {
		ID    int64    `json:"id"`
		Roles []string `json:"roles"`
		OK    bool     `json:"ok"`
		CtxOK bool     `json:"ctxOK"`
		CtxID int64    `json:"ctxID"`
	}
	r.Decode(&me)
	if r.Status != 200 || me.ID != 42 || len(me.Roles) != 2 || !me.OK || !me.CtxOK || me.CtxID != 42 {
		t.Fatalf("identity not propagated: %s", r.Body)
	}

	// 伪造：只带 X-User-Id（没有签名）——绕过网关直连时的典型攻击。
	spoof := testkit.DoJSON(t, e, "GET", "/api/v1/me", nil, testkit.Header("X-User-Id", "1"), testkit.Header("X-User-Roles", "admin"))
	if spoof.Status != 401 || spoof.Code != apperr.CodeAuthFail {
		t.Fatalf("unsigned identity headers must be rejected: %d %s", spoof.Status, spoof.Body)
	}
	// 篡改：改用户 ID 但沿用旧签名。
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	testkit.AsUser(req, 42, "user")
	req.Header.Set("X-User-Id", "1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("tampered identity must be rejected, got %d", rec.Code)
	}
	// 过期：签名有效但时间戳超出 ±60s（重放）。
	stale := httptest.NewRequest("GET", "/api/v1/me", nil)
	testkit.AsUser(stale, 42, "user")
	clk.Advance(2 * time.Minute)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, stale)
	if rec.Code != 401 {
		t.Fatalf("replayed (stale) identity must be rejected, got %d", rec.Code)
	}
	// 无头。
	if r := testkit.DoJSON(t, e, "GET", "/api/v1/me", nil); r.Status != 401 {
		t.Fatalf("anonymous request must be 401, got %d", r.Status)
	}
}

func TestRequireRole(t *testing.T) {
	e, _ := newEngine(t)
	g := e.Group("/api/v1", httpx.RequireGatewayIdentity(testkit.Signer(t)))
	g.GET("/admin", httpx.RequireRole("admin"), func(c *gin.Context) { httpx.OK(c, nil) })
	g.GET("/either", httpx.RequireRole("admin", "ops"), func(c *gin.Context) { httpx.OK(c, nil) })
	e.GET("/unguarded", httpx.RequireRole("admin"), func(c *gin.Context) { httpx.OK(c, nil) })

	if r := testkit.DoJSON(t, e, "GET", "/api/v1/admin", nil, testkit.User(1, "admin")); r.Status != 200 {
		t.Fatalf("admin rejected: %s", r.Body)
	}
	if r := testkit.DoJSON(t, e, "GET", "/api/v1/admin", nil, testkit.User(2, "user")); r.Status != 403 || r.Code != apperr.CodeForbidden {
		t.Fatalf("plain user must get 403/20006, got %d/%d", r.Status, r.Code)
	}
	if r := testkit.DoJSON(t, e, "GET", "/api/v1/admin", nil, testkit.User(3)); r.Status != 403 {
		t.Fatalf("role-less user must get 403, got %d", r.Status)
	}
	if r := testkit.DoJSON(t, e, "GET", "/api/v1/either", nil, testkit.User(4, "ops")); r.Status != 200 {
		t.Fatal("any-of semantics expected")
	}
	if r := testkit.DoJSON(t, e, "GET", "/unguarded", nil); r.Status != 401 {
		t.Fatalf("RequireRole without an identity must fail closed with 401, got %d", r.Status)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	l := limiter.NewBuckets(limiter.BucketConfig{Rate: 1, Burst: 2, Now: clk.Now})
	e, _ := newEngine(t)
	e.GET("/x", httpx.RateLimit(l, func(c *gin.Context) string { return c.GetHeader("X-Key") }), func(c *gin.Context) { httpx.OK(c, nil) })

	k := testkit.Header("X-Key", "ip-1")
	for i := 0; i < 2; i++ {
		if r := testkit.DoJSON(t, e, "GET", "/x", nil, k); r.Status != 200 {
			t.Fatalf("request %d within burst rejected", i)
		}
	}
	r := testkit.DoJSON(t, e, "GET", "/x", nil, k)
	if r.Status != 429 || r.Code != apperr.CodeRateLimited || r.Header.Get("Retry-After") != "1" {
		t.Fatalf("over-limit must be 429/10003 with Retry-After, got %d/%d retry=%q", r.Status, r.Code, r.Header.Get("Retry-After"))
	}
	if r := testkit.DoJSON(t, e, "GET", "/x", nil, testkit.Header("X-Key", "ip-2")); r.Status != 200 {
		t.Fatal("other keys must be unaffected")
	}
	if r := testkit.DoJSON(t, e, "GET", "/x", nil); r.Status != 200 {
		t.Fatal("an empty key means no limiting")
	}
	clk.Advance(time.Second)
	if r := testkit.DoJSON(t, e, "GET", "/x", nil, k); r.Status != 200 {
		t.Fatal("tokens must refill over time")
	}
}

func TestTrustedProxiesDefaultIgnoresForwardedFor(t *testing.T) {
	e, _ := newEngine(t)
	var seen string
	e.GET("/ip", func(c *gin.Context) { seen = c.ClientIP(); httpx.OK(c, nil) })
	testkit.DoJSON(t, e, "GET", "/ip", nil, testkit.Header("X-Forwarded-For", "6.6.6.6"), testkit.RemoteAddr("10.1.2.3:5555"))
	if seen != "10.1.2.3" {
		t.Fatalf("by default X-Forwarded-For must be ignored (rate-limit bypass), got %q", seen)
	}
	trusted, _ := newEngine(t, func(o *httpx.Options) { o.TrustedProxies = []string{"10.0.0.0/8"} })
	trusted.GET("/ip", func(c *gin.Context) { seen = c.ClientIP(); httpx.OK(c, nil) })
	testkit.DoJSON(t, trusted, "GET", "/ip", nil, testkit.Header("X-Forwarded-For", "6.6.6.6"), testkit.RemoteAddr("10.1.2.3:5555"))
	if seen != "6.6.6.6" {
		t.Fatalf("with a trusted proxy XFF must be honoured, got %q", seen)
	}
}
