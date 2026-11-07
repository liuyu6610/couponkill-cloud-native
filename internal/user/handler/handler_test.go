package handler_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/handler"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/usertest"
)

// 每个响应体都会被登记下来，测试结束时统一断言没有任何口令哈希或口令字段泄露。
var leakMarkers = []string{"$2a$", "$2b$", "passwordHash", "password_hash", "PasswordHash", "\"password\""}

type server struct {
	t        *testing.T
	h        http.Handler
	repo     *usertest.MemRepo
	sessions *authx.MemoryRevoker
	tokens   *authx.Tokens

	mu     sync.Mutex
	bodies [][]byte
}

func newServer(t *testing.T) *server {
	t.Helper()
	repo := usertest.NewMemRepo(nil)
	sessions := authx.NewMemoryRevoker()
	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: testkit.JWTSecret, Issuer: "couponkill", Audience: "couponkill-api", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	var seq int64 = 1000
	svc, err := service.New(service.Deps{
		Repo: repo, Sessions: sessions, Tokens: tokens, IDs: idFunc(func() int64 { seq++; return seq }),
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	eng := httpx.NewEngine(httpx.Options{Service: "user", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	handler.Mount(eng.Engine, handler.Options{Service: svc, Signer: testkit.Signer(t), InternalToken: testkit.InternalToken})

	s := &server{t: t, h: eng, repo: repo, sessions: sessions, tokens: tokens}
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, b := range s.bodies {
			for _, m := range leakMarkers {
				if bytes.Contains(b, []byte(m)) {
					t.Errorf("响应体泄露了 %q: %s", m, b)
				}
			}
		}
	})
	return s
}

type idFunc func() int64

func (f idFunc) Next() int64 { return f() }

func (s *server) do(method, path string, body any, opts ...testkit.ReqOption) *testkit.Response {
	s.t.Helper()
	res := testkit.DoJSON(s.t, s.h, method, path, body, opts...)
	s.mu.Lock()
	s.bodies = append(s.bodies, res.Body)
	s.mu.Unlock()
	return res
}

func (s *server) register(name string) *testkit.Response {
	s.t.Helper()
	return s.do("POST", "/api/v1/user/register", map[string]string{"username": name, "password": "secret1", "phone": "13800000000"})
}

// loginToken 登录并返回令牌原文与 userId。
func (s *server) loginToken(name, password string) (token string, userID int64) {
	s.t.Helper()
	res := s.do("POST", "/api/v1/user/login", map[string]string{"username": name, "password": password})
	if res.Status != 200 {
		s.t.Fatalf("login %s: status %d body %s", name, res.Status, res.Body)
	}
	var out struct {
		Token  string `json:"token"`
		UserID string `json:"userId"`
	}
	res.Decode(&out)
	sess, err := s.tokens.Verify(out.Token)
	if err != nil {
		s.t.Fatalf("issued token invalid: %v", err)
	}
	return out.Token, sess.Identity.UserID
}

func bearer(token string) testkit.ReqOption { return testkit.Header("Authorization", "Bearer "+token) }

// ---- register ----

func TestRegisterSuccessEnvelope(t *testing.T) {
	s := newServer(t)
	res := s.register("alice")
	if res.Status != 200 || res.Code != 200 || res.Message != "ok" {
		t.Fatalf("status=%d code=%d msg=%q body=%s", res.Status, res.Code, res.Message, res.Body)
	}
	var out map[string]any
	res.Decode(&out)
	if id, ok := out["id"].(string); !ok || id == "" {
		t.Fatalf("id 必须是字符串: %v", out["id"])
	}
	if out["username"] != "alice" || out["phone"] != "13800000000" || out["role"] != "user" {
		t.Fatalf("data = %v", out)
	}
	for _, k := range []string{"password", "passwordHash", "password_hash"} {
		if _, ok := out[k]; ok {
			t.Fatalf("响应不得含 %s", k)
		}
	}
}

func TestRegisterValidationFailuresAre400WithMessage(t *testing.T) {
	s := newServer(t)
	tests := []struct {
		name string
		body any
		want string
	}{
		{"short password", map[string]string{"username": "a", "password": "123", "phone": "13800000000"}, "密码"},
		{"bad phone", map[string]string{"username": "a", "password": "secret1", "phone": "123"}, "手机号"},
		{"missing username", map[string]string{"password": "secret1", "phone": "13800000000"}, "用户名"},
		{"empty object", map[string]string{}, "用户名"},
		{"invalid json", "{not json", "请求参数错误"},
		{"empty body", "", "请求参数错误"},
		{"wrong type", `{"username":"a","password":123,"phone":"13800000000"}`, "请求参数错误"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := s.do("POST", "/api/v1/user/register", tc.body)
			if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
				t.Fatalf("status=%d code=%d body=%s", res.Status, res.Code, res.Body)
			}
			if !strings.Contains(res.Message, tc.want) {
				t.Fatalf("message %q should mention %q", res.Message, tc.want)
			}
			if !res.DataIsNull() {
				t.Fatal("失败响应 data 必须为 null")
			}
		})
	}
	if s.repo.Len() != 0 {
		t.Fatal("校验失败不得创建用户")
	}
}

func TestRegisterDuplicateUsernameIs409(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	res := s.register("alice")
	if res.Status != 409 || res.Code != apperr.CodeUserExist {
		t.Fatalf("status=%d code=%d body=%s", res.Status, res.Code, res.Body)
	}
}

func TestRegisterIgnoresClientSuppliedRole(t *testing.T) {
	s := newServer(t)
	res := s.do("POST", "/api/v1/user/register",
		`{"username":"mallory","password":"secret1","phone":"13800000000","role":"admin","status":0,"id":"1"}`)
	if res.Status != 200 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var out struct {
		ID     string `json:"id"`
		Role   string `json:"role"`
		Status int    `json:"status"`
	}
	res.Decode(&out)
	if out.Role != "user" || out.Status != 1 || out.ID == "1" {
		t.Fatalf("角色/状态/ID 只能由服务端决定: %+v", out)
	}
}

func TestRegisterRejectsPasswordInQueryString(t *testing.T) {
	s := newServer(t)
	for _, q := range []string{"password=secret1", "username=alice&password=secret1", "pass%77ord=secret1", "PASSWORD=secret1"} {
		res := s.do("POST", "/api/v1/user/register?"+q,
			map[string]string{"username": "alice", "password": "secret1", "phone": "13800000000"})
		if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
			t.Fatalf("?%s: status=%d body=%s", q, res.Status, res.Body)
		}
		if !strings.Contains(res.Message, "URL") {
			t.Fatalf("?%s: message %q 应说明不能在 URL 里传密码", q, res.Message)
		}
	}
	if s.repo.Len() != 0 {
		t.Fatal("查询串带口令的请求不得创建用户")
	}
}

// ---- login ----

func TestLoginSuccessShape(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	res := s.do("POST", "/api/v1/user/login", map[string]string{"username": "alice", "password": "secret1"})
	if res.Status != 200 || res.Code != 200 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, 令牌响应必须 no-store", cc)
	}
	var out struct {
		Token     string   `json:"token"`
		UserID    string   `json:"userId"`
		Username  string   `json:"username"`
		Roles     []string `json:"roles"`
		ExpiresAt string   `json:"expiresAt"`
	}
	res.Decode(&out)
	if out.Token == "" || out.UserID == "" || out.Username != "alice" || len(out.Roles) != 1 || out.Roles[0] != "user" {
		t.Fatalf("data = %+v", out)
	}
	exp, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil || time.Until(exp) < 50*time.Minute || time.Until(exp) > 61*time.Minute {
		t.Fatalf("expiresAt = %q (%v)", out.ExpiresAt, err)
	}
	sess, err := s.tokens.Verify(out.Token)
	if err != nil {
		t.Fatalf("token should verify: %v", err)
	}
	if on, _ := s.sessions.Active(t.Context(), sess.JTI); !on {
		t.Fatal("登录应登记会话")
	}
}

func TestLoginWrongPasswordAndUnknownUserGiveIdenticalResponses(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	wrong := s.do("POST", "/api/v1/user/login", map[string]string{"username": "alice", "password": "nope-nope"})
	unknown := s.do("POST", "/api/v1/user/login", map[string]string{"username": "ghost", "password": "nope-nope"})
	if wrong.Status != 401 || unknown.Status != 401 {
		t.Fatalf("status wrong=%d unknown=%d, want 401", wrong.Status, unknown.Status)
	}
	if wrong.Code != apperr.CodePasswordError || wrong.Code != unknown.Code || wrong.Message != unknown.Message {
		t.Fatalf("响应不一致: %d/%q vs %d/%q", wrong.Code, wrong.Message, unknown.Code, unknown.Message)
	}
	if wrong.Message != "用户名或密码错误" {
		t.Fatalf("message = %q", wrong.Message)
	}
	// 去掉 requestId 之后整个响应体逐字节相同。
	strip := func(r *testkit.Response) string {
		return strings.ReplaceAll(string(r.Body), r.RequestID, "")
	}
	if strip(wrong) != strip(unknown) {
		t.Fatalf("响应体不同:\n%s\n%s", wrong.Body, unknown.Body)
	}
}

func TestLoginMissingFieldsAre400(t *testing.T) {
	s := newServer(t)
	for _, body := range []any{
		map[string]string{"username": "alice"},
		map[string]string{"password": "secret1"},
		map[string]string{},
		"not json",
		"",
	} {
		res := s.do("POST", "/api/v1/user/login", body)
		if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
			t.Fatalf("body %v: status=%d code=%d %s", body, res.Status, res.Code, res.Body)
		}
	}
}

func TestLoginCredentialsInQueryStringAreRejectedAndNeverUsed(t *testing.T) {
	s := newServer(t)
	s.register("alice")

	// 1) 只有查询串、没有请求体：不能因为查询串里的凭证而登录成功。
	res := s.do("POST", "/api/v1/user/login?username=alice&password=secret1", nil)
	if res.Status != 400 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	if strings.Contains(string(res.Body), `"token"`) {
		t.Fatal("不得签发令牌")
	}
	// 2) 请求体凭证正确、但查询串里也带了口令：同样拒绝（提示客户端改用请求体）。
	res = s.do("POST", "/api/v1/user/login?password=secret1", map[string]string{"username": "alice", "password": "secret1"})
	if res.Status != 400 || !strings.Contains(res.Message, "URL") {
		t.Fatalf("status=%d msg=%q", res.Status, res.Message)
	}
	// 3) 只带 username 的查询串不会被当作凭证使用：体里的口令错误仍然失败。
	res = s.do("POST", "/api/v1/user/login?username=alice", map[string]string{"username": "ghost", "password": "secret1"})
	if res.Status != 401 {
		t.Fatalf("查询串里的 username 不得覆盖请求体: status=%d", res.Status)
	}
}

func TestLoginRejectsGetAndFormEncoded(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	if res := s.do("GET", "/api/v1/user/login?username=alice&password=secret1", nil); res.Status != 404 && res.Status != 405 {
		t.Fatalf("GET /login status = %d", res.Status)
	}
	req := httptest.NewRequest("POST", "/api/v1/user/login", strings.NewReader("username=alice&password=secret1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("表单登录不再支持，status = %d", rec.Code)
	}
}

func TestNoMockTokenEndpoint(t *testing.T) {
	s := newServer(t)
	for _, p := range []string{"/api/v1/auth/token/mock", "/api/v1/user/token/mock", "/api/v1/user/mock-token"} {
		for _, m := range []string{"GET", "POST"} {
			if res := s.do(m, p, nil); res.Status != 404 && res.Status != 405 {
				t.Fatalf("%s %s status = %d, mock-token 接口不应存在", m, p, res.Status)
			}
		}
	}
}

// ---- logout ----

func TestLogoutRequiresGatewayIdentity(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	token, _ := s.loginToken("alice", "secret1")

	res := s.do("POST", "/api/v1/user/logout", nil, bearer(token))
	if res.Status != 401 || res.Code != apperr.CodeAuthFail {
		t.Fatalf("没有网关身份头必须 401: status=%d body=%s", res.Status, res.Body)
	}
	// 伪造的身份头（无签名）同样被拒。
	res = s.do("POST", "/api/v1/user/logout", nil, bearer(token), testkit.Header("X-User-Id", "1001"), testkit.Header("X-User-Roles", "admin"))
	if res.Status != 401 {
		t.Fatalf("伪造身份头 status = %d, want 401", res.Status)
	}
}

func TestLogoutRevokesCurrentTokenOnlyAndIsIdempotent(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	t1, uid := s.loginToken("alice", "secret1")
	t2, _ := s.loginToken("alice", "secret1")
	s1, _ := s.tokens.Verify(t1)
	s2, _ := s.tokens.Verify(t2)

	for i := 0; i < 2; i++ {
		res := s.do("POST", "/api/v1/user/logout", nil, testkit.User(uid, "user"), bearer(t1))
		if res.Status != 200 || res.Code != 200 || !res.DataIsNull() {
			t.Fatalf("logout #%d: status=%d body=%s", i+1, res.Status, res.Body)
		}
	}
	if on, _ := s.sessions.Active(t.Context(), s1.JTI); on {
		t.Fatal("当前令牌的会话应被吊销")
	}
	if on, _ := s.sessions.Active(t.Context(), s2.JTI); !on {
		t.Fatal("另一个登录会话不得受影响")
	}
}

func TestLogoutWithoutOrWithBadBearerIs401(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	_, uid := s.loginToken("alice", "secret1")
	for name, opts := range map[string][]testkit.ReqOption{
		"no authorization":  {testkit.User(uid, "user")},
		"not bearer":        {testkit.User(uid, "user"), testkit.Header("Authorization", "Basic abc")},
		"garbage bearer":    {testkit.User(uid, "user"), bearer("garbage")},
		"empty bearer":      {testkit.User(uid, "user"), testkit.Header("Authorization", "Bearer ")},
		"lowercase scheme":  {testkit.User(uid, "user"), testkit.Header("Authorization", "bearer garbage")},
		"bearer no space":   {testkit.User(uid, "user"), testkit.Header("Authorization", "Bearergarbage")},
		"bearer extra part": {testkit.User(uid, "user"), testkit.Header("Authorization", "Bearer a b")},
	} {
		res := s.do("POST", "/api/v1/user/logout", nil, opts...)
		if res.Status != 401 || res.Code != apperr.CodeTokenInvalid {
			t.Fatalf("%s: status=%d code=%d body=%s", name, res.Status, res.Code, res.Body)
		}
	}
}

func TestLogoutWithAnotherUsersTokenIs401AndKeepsSession(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	s.register("bob")
	aliceToken, _ := s.loginToken("alice", "secret1")
	_, bobID := s.loginToken("bob", "secret1")
	sess, _ := s.tokens.Verify(aliceToken)

	res := s.do("POST", "/api/v1/user/logout", nil, testkit.User(bobID, "user"), bearer(aliceToken))
	if res.Status != 401 {
		t.Fatalf("status = %d, want 401", res.Status)
	}
	if on, _ := s.sessions.Active(t.Context(), sess.JTI); !on {
		t.Fatal("alice 的会话不得被 bob 吊销")
	}
}

// ---- profile ----

func TestProfileRequiresIdentity(t *testing.T) {
	s := newServer(t)
	res := s.do("GET", "/api/v1/user/profile", nil)
	if res.Status != 401 || res.Code != apperr.CodeAuthFail {
		t.Fatalf("status=%d code=%d", res.Status, res.Code)
	}
}

func TestProfileReturnsCurrentUserWithoutHash(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	s.register("bob")
	_, aliceID := s.loginToken("alice", "secret1")

	res := s.do("GET", "/api/v1/user/profile", nil, testkit.User(aliceID, "user"))
	if res.Status != 200 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var out map[string]any
	res.Decode(&out)
	if out["username"] != "alice" || out["role"] != "user" || out["phone"] != "13800000000" {
		t.Fatalf("data = %v", out)
	}
	if _, ok := out["id"].(string); !ok {
		t.Fatalf("id 必须是字符串: %v", out["id"])
	}
	if _, ok := out["lastActiveAt"].(string); !ok {
		t.Fatalf("登录后应有 lastActiveAt: %v", out)
	}
	for _, k := range []string{"createdAt", "updatedAt"} {
		if _, ok := out[k].(string); !ok {
			t.Fatalf("前端按 %s 读取时间字段: %v", k, out)
		}
	}
	for _, gone := range []string{"createTime", "updateTime", "lastActiveTime"} {
		if _, ok := out[gone]; ok {
			t.Fatalf("Java 时代的字段名 %s 不应再输出: %v", gone, out)
		}
	}
	// 查询串里的 userId 不得改变查询对象：身份只来自网关。
	res = s.do("GET", "/api/v1/user/profile?userId=1001&id=1002", nil, testkit.User(aliceID, "user"))
	var again map[string]any
	res.Decode(&again)
	if again["username"] != "alice" {
		t.Fatalf("profile 只能返回当前用户: %v", again)
	}
}

func TestProfileOfDeletedUserIs404(t *testing.T) {
	s := newServer(t)
	res := s.do("GET", "/api/v1/user/profile", nil, testkit.User(424242, "user"))
	if res.Status != 404 || res.Code != apperr.CodeUserNotFound {
		t.Fatalf("status=%d code=%d", res.Status, res.Code)
	}
}

// ---- batch/generate ----

func TestBatchGenerateAuthorization(t *testing.T) {
	s := newServer(t)
	body := map[string]any{"prefix": "lt", "count": 3}

	if res := s.do("POST", "/api/v1/user/batch/generate", body); res.Status != 401 {
		t.Fatalf("无身份 status = %d, want 401", res.Status)
	}
	res := s.do("POST", "/api/v1/user/batch/generate", body, testkit.User(7, "user"))
	if res.Status != 403 || res.Code != apperr.CodeForbidden {
		t.Fatalf("普通用户 status=%d code=%d, want 403", res.Status, res.Code)
	}
	if s.repo.Len() != 0 {
		t.Fatal("非管理员请求不得创建任何用户")
	}
}

func TestBatchGenerateAsAdminAndRerun(t *testing.T) {
	s := newServer(t)
	body := map[string]any{"prefix": "lt", "start": 0, "count": 25}
	admin := testkit.User(1, "admin", "user")

	res := s.do("POST", "/api/v1/user/batch/generate", body, admin)
	if res.Status != 200 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var out struct{ Requested, Created, Existing int }
	res.Decode(&out)
	if out.Requested != 25 || out.Created != 25 || out.Existing != 0 {
		t.Fatalf("out = %+v", out)
	}
	res = s.do("POST", "/api/v1/user/batch/generate", body, admin)
	res.Decode(&out)
	if res.Status != 200 || out.Created != 0 || out.Existing != 25 {
		t.Fatalf("重跑: status=%d out=%+v", res.Status, out)
	}
	if s.repo.Len() != 25 {
		t.Fatalf("users = %d, want 25", s.repo.Len())
	}
	// 造出来的用户能用默认口令登录。
	if token, _ := s.loginToken("lt7", service.DefaultBatchPassword); token == "" {
		t.Fatal("批量用户应能登录")
	}
}

func TestBatchGenerateRejectsBadInputWith400(t *testing.T) {
	s := newServer(t)
	admin := testkit.User(1, "admin")
	for name, body := range map[string]any{
		"count zero":     map[string]any{"prefix": "lt", "count": 0},
		"count too big":  map[string]any{"prefix": "lt", "count": 100001},
		"no prefix":      map[string]any{"count": 5},
		"bad prefix":     map[string]any{"prefix": "a b", "count": 5},
		"negative start": map[string]any{"prefix": "lt", "start": -1, "count": 5},
		"invalid json":   "{",
		"string count":   `{"prefix":"lt","count":"5"}`,
	} {
		res := s.do("POST", "/api/v1/user/batch/generate", body, admin)
		if res.Status != 400 || res.Code != apperr.CodeInvalidRequest {
			t.Fatalf("%s: status=%d body=%s", name, res.Status, res.Body)
		}
	}
	if s.repo.Len() != 0 {
		t.Fatal("非法请求不得创建用户")
	}
}

func TestBatchGenerateDoesNotAcceptParamsFromQueryString(t *testing.T) {
	s := newServer(t)
	res := s.do("POST", "/api/v1/user/batch/generate?prefix=lt&count=5", nil, testkit.User(1, "admin"))
	if res.Status != 400 {
		t.Fatalf("参数只认 JSON 请求体: status=%d", res.Status)
	}
}

// ---- internal ----

func TestInternalUserLookup(t *testing.T) {
	s := newServer(t)
	s.register("alice")
	_, uid := s.loginToken("alice", "secret1")
	path := "/internal/v1/users/" + itoa(uid)

	if res := s.do("GET", path, nil); res.Status != 401 {
		t.Fatalf("无内部令牌 status = %d, want 401", res.Status)
	}
	if res := s.do("GET", path, nil, testkit.Header("X-Internal-Token", "wrong")); res.Status != 401 {
		t.Fatalf("错误内部令牌 status = %d, want 401", res.Status)
	}
	// 用户身份头不能代替内部令牌。
	if res := s.do("GET", path, nil, testkit.User(uid, "admin")); res.Status != 401 {
		t.Fatalf("网关身份不能访问内部端点 status = %d", res.Status)
	}

	res := s.do("GET", path, nil, testkit.Internal())
	if res.Status != 200 {
		t.Fatalf("status=%d body=%s", res.Status, res.Body)
	}
	var out map[string]any
	res.Decode(&out)
	if out["id"] != itoa(uid) || out["username"] != "alice" || out["role"] != "user" || out["status"] != float64(1) {
		t.Fatalf("data = %v", out)
	}
	if len(out) != 4 {
		t.Fatalf("internalapi.UserInfo 只有 id/username/role/status 四个字段: %v", out)
	}
}

func TestInternalUserLookupErrors(t *testing.T) {
	s := newServer(t)
	if res := s.do("GET", "/internal/v1/users/424242", nil, testkit.Internal()); res.Status != 404 || res.Code != apperr.CodeUserNotFound {
		t.Fatalf("status=%d code=%d", res.Status, res.Code)
	}
	for _, id := range []string{"abc", "0", "-5", "1.5"} {
		if res := s.do("GET", "/internal/v1/users/"+id, nil, testkit.Internal()); res.Status != 400 {
			t.Fatalf("id %q: status = %d, want 400", id, res.Status)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
