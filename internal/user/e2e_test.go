package user_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/redisx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// stack 是一整套真实装配：PostgreSQL（一次性库）+ miniredis + user 服务 + 一个只做验令牌/查会话/签身份的迷你网关。
type stack struct {
	t        *testing.T
	srv      *user.Server
	pool     *pgxpool.Pool
	rdb      *redis.Client
	mr       *miniredis.Miniredis
	tokens   *authx.Tokens
	sessions *redisx.SessionStore
	signer   *authx.GatewaySigner
}

func newStack(t *testing.T, cfg user.Config) *stack {
	t.Helper()
	pool := dbxtest.New(t)
	if err := user.Bootstrap(context.Background(), pool, cfg, quietLogger()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	rdb, mr := testkit.Redis(t)
	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: testkit.JWTSecret, Issuer: "couponkill", Audience: "couponkill-api", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	ids, err := idgen.New(7)
	if err != nil {
		t.Fatalf("idgen: %v", err)
	}
	signer := testkit.Signer(t)
	srv, err := user.New(user.Options{
		Pool: pool, Redis: rdb, Tokens: tokens, Signer: signer,
		InternalToken: testkit.InternalToken, IDs: ids, Logger: quietLogger(),
	})
	if err != nil {
		t.Fatalf("user.New: %v", err)
	}
	return &stack{t: t, srv: srv, pool: pool, rdb: rdb, mr: mr, tokens: tokens, sessions: redisx.NewSessionStore(rdb), signer: signer}
}

// gateway 模拟网关对一个携带 Bearer 令牌的请求所做的事：验 JWT、查会话是否有效、
// 通过后签名并转发身份头（Authorization 原样透传）。会话无效时返回 false。
func (s *stack) gateway(token string) testkit.ReqOption {
	s.t.Helper()
	sess, err := s.tokens.Verify(token)
	if err != nil {
		s.t.Fatalf("gateway: verify token: %v", err)
	}
	active, err := s.sessions.Active(context.Background(), sess.JTI)
	if err != nil {
		s.t.Fatalf("gateway: check session: %v", err)
	}
	if !active {
		return func(*http.Request) {} // 无身份头，等价于被网关拒绝后直连：下游 401
	}
	return func(r *http.Request) {
		if err := s.signer.Apply(r.Header, sess.Identity); err != nil {
			s.t.Fatalf("gateway: apply identity: %v", err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
	}
}

func (s *stack) sessionActive(token string) bool {
	s.t.Helper()
	sess, err := s.tokens.Verify(token)
	if err != nil {
		s.t.Fatalf("verify: %v", err)
	}
	ok, err := s.sessions.Active(context.Background(), sess.JTI)
	if err != nil {
		s.t.Fatalf("Active: %v", err)
	}
	return ok
}

func (s *stack) do(method, path string, body any, opts ...testkit.ReqOption) *testkit.Response {
	s.t.Helper()
	return testkit.DoJSON(s.t, s.srv.Engine, method, path, body, opts...)
}

func (s *stack) login(username, password string) (token string, res *testkit.Response) {
	s.t.Helper()
	res = s.do("POST", "/api/v1/user/login", map[string]string{"username": username, "password": password})
	if res.Status != 200 {
		return "", res
	}
	var out struct {
		Token string `json:"token"`
	}
	res.Decode(&out)
	return out.Token, res
}

func TestEndToEndRegisterLoginProfileLogout(t *testing.T) {
	s := newStack(t, user.Config{})

	// 注册
	reg := s.do("POST", "/api/v1/user/register", map[string]string{"username": "carol", "password": "secret1", "phone": "13900000001"})
	if reg.Status != 200 {
		t.Fatalf("register: status=%d body=%s", reg.Status, reg.Body)
	}
	var registered struct{ ID string }
	reg.Decode(&registered)

	// 登录：令牌可验、会话落到 Redis、TTL=令牌寿命
	token, res := s.login("carol", "secret1")
	if token == "" {
		t.Fatalf("login: status=%d body=%s", res.Status, res.Body)
	}
	sess, err := s.tokens.Verify(token)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if strconv.FormatInt(sess.Identity.UserID, 10) != registered.ID {
		t.Fatalf("token sub %d != registered id %s", sess.Identity.UserID, registered.ID)
	}
	if !s.mr.Exists("ck:sess:" + sess.JTI) {
		t.Fatal("登录后 Redis 里应有 ck:sess:{jti}")
	}
	if ttl := s.mr.TTL("ck:sess:" + sess.JTI); ttl < 59*time.Minute || ttl > time.Hour {
		t.Fatalf("会话 TTL = %v, want ≈1h", ttl)
	}
	if !s.sessionActive(token) {
		t.Fatal("网关视角：登录后令牌应有效")
	}

	// 经网关签名访问 profile
	prof := s.do("GET", "/api/v1/user/profile", nil, s.gateway(token))
	if prof.Status != 200 {
		t.Fatalf("profile: status=%d body=%s", prof.Status, prof.Body)
	}
	var p struct {
		ID, Username, Phone, Role string
		LastActiveAt              string
	}
	prof.Decode(&p)
	if p.Username != "carol" || p.Phone != "13900000001" || p.Role != "user" || p.ID != registered.ID || p.LastActiveAt == "" {
		t.Fatalf("profile = %+v", p)
	}

	// 另开一个会话，登出第一个只影响第一个
	token2, _ := s.login("carol", "secret1")
	out := s.do("POST", "/api/v1/user/logout", nil, s.gateway(token))
	if out.Status != 200 {
		t.Fatalf("logout: status=%d body=%s", out.Status, out.Body)
	}
	if s.sessionActive(token) {
		t.Fatal("登出后该令牌的会话不应再有效（网关会拒绝它）")
	}
	if s.mr.Exists("ck:sess:" + sess.JTI) {
		t.Fatal("Redis 里的会话键应被删除")
	}
	if !s.sessionActive(token2) {
		t.Fatal("同一用户的另一个会话不得被吊销")
	}
	// 被吊销的令牌再走网关：拿不到身份头，下游 401
	if again := s.do("GET", "/api/v1/user/profile", nil, s.gateway(token)); again.Status != 401 {
		t.Fatalf("登出后的令牌访问 profile status = %d, want 401", again.Status)
	}
	// 重新登录不会让旧令牌复活；登出幂等。
	s.login("carol", "secret1")
	if s.sessionActive(token) {
		t.Fatal("重新登录不得复活已登出的令牌")
	}
	if res := s.do("POST", "/api/v1/user/logout", nil, testkit.User(sess.Identity.UserID, "user"), testkit.Header("Authorization", "Bearer "+token)); res.Status != 200 {
		t.Fatalf("重复登出应幂等成功: status=%d body=%s", res.Status, res.Body)
	}
}

func TestEndToEndDuplicateAndUnknownUserBehaviour(t *testing.T) {
	s := newStack(t, user.Config{})
	body := map[string]string{"username": "dave", "password": "secret1", "phone": "13900000002"}
	if res := s.do("POST", "/api/v1/user/register", body); res.Status != 200 {
		t.Fatalf("first register: %d %s", res.Status, res.Body)
	}
	dup := s.do("POST", "/api/v1/user/register", body)
	if dup.Status != 409 || dup.Code != apperr.CodeUserExist {
		t.Fatalf("duplicate: status=%d code=%d", dup.Status, dup.Code)
	}
	_, wrong := s.login("dave", "bad-password")
	_, unknown := s.login("nobody", "bad-password")
	if wrong.Status != 401 || unknown.Status != 401 || wrong.Message != unknown.Message || wrong.Code != unknown.Code {
		t.Fatalf("wrong=%d/%q unknown=%d/%q", wrong.Status, wrong.Message, unknown.Status, unknown.Message)
	}
}

func TestEndToEndConcurrentSameUsernameRegistrationExactlyOneWins(t *testing.T) {
	s := newStack(t, user.Config{})
	const n = 20
	statuses := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res := s.do("POST", "/api/v1/user/register", map[string]string{"username": "race", "password": "secret1", "phone": "13900000003"})
			statuses[i] = res.Status
		}()
	}
	close(start)
	wg.Wait()
	ok, conflict := 0, 0
	for _, st := range statuses {
		switch st {
		case 200:
			ok++
		case 409:
			conflict++
		default:
			t.Fatalf("unexpected status %d in %v", st, statuses)
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("ok=%d conflict=%d, want 1/%d", ok, conflict, n-1)
	}
	var rows int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM usr.users WHERE username='race'").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows = %d err=%v", rows, err)
	}
}

func TestEndToEndDemoSeedAdminBatchGenerateAndInternalLookup(t *testing.T) {
	s := newStack(t, user.Config{UserSeedDemo: true})

	userToken, res := s.login("demo", "demo1234")
	if userToken == "" {
		t.Fatalf("demo login: %d %s", res.Status, res.Body)
	}
	adminToken, res := s.login("admin", "admin1234")
	if adminToken == "" {
		t.Fatalf("admin login: %d %s", res.Status, res.Body)
	}
	adminSess, _ := s.tokens.Verify(adminToken)
	if !adminSess.Identity.IsAdmin() {
		t.Fatalf("admin 令牌应带 admin 角色: %v", adminSess.Identity.Roles)
	}

	body := map[string]any{"prefix": "e2e", "count": 50}
	if r := s.do("POST", "/api/v1/user/batch/generate", body, s.gateway(userToken)); r.Status != 403 {
		t.Fatalf("普通用户造用户 status = %d, want 403", r.Status)
	}
	r := s.do("POST", "/api/v1/user/batch/generate", body, s.gateway(adminToken))
	if r.Status != 200 {
		t.Fatalf("admin batch: %d %s", r.Status, r.Body)
	}
	var out struct{ Created, Existing int }
	r.Decode(&out)
	if out.Created != 50 || out.Existing != 0 {
		t.Fatalf("out = %+v", out)
	}
	if tk, res := s.login("e2e49", "password123"); tk == "" {
		t.Fatalf("批量用户应能登录: %d %s", res.Status, res.Body)
	}

	// 内部端点
	demoSess, _ := s.tokens.Verify(userToken)
	r = s.do("GET", "/internal/v1/users/"+strconv.FormatInt(demoSess.Identity.UserID, 10), nil, testkit.Internal())
	var info struct {
		ID, Username, Role string
		Status             int
	}
	r.Decode(&info)
	if r.Status != 200 || info.Username != "demo" || info.Role != "user" || info.Status != 1 {
		t.Fatalf("internal lookup: %d %+v", r.Status, info)
	}
}

func TestBootstrapSeedsOnlyWhenRequestedAndNeverInProd(t *testing.T) {
	count := func(pool *pgxpool.Pool) int {
		var n int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM usr.users").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	ctx := context.Background()

	t.Run("no seed by default", func(t *testing.T) {
		pool := dbxtest.New(t)
		if err := user.Bootstrap(ctx, pool, user.Config{}, quietLogger()); err != nil {
			t.Fatal(err)
		}
		if n := count(pool); n != 0 {
			t.Fatalf("users = %d, want 0", n)
		}
	})
	for name, cfg := range map[string]user.Config{
		"CK_USER_SEED_DEMO": {UserSeedDemo: true},
		"CK_SEED_DEMO":      {SeedDemo: true},
	} {
		t.Run("seed via "+name+" is idempotent", func(t *testing.T) {
			pool := dbxtest.New(t)
			for i := 0; i < 2; i++ {
				if err := user.Bootstrap(ctx, pool, cfg, quietLogger()); err != nil {
					t.Fatalf("Bootstrap #%d: %v", i+1, err)
				}
			}
			if n := count(pool); n != 2 {
				t.Fatalf("users = %d, want 2", n)
			}
		})
	}
	t.Run("prod refuses seed", func(t *testing.T) {
		pool := dbxtest.New(t)
		cfg := user.Config{UserSeedDemo: true}
		cfg.Env = "prod"
		err := user.Bootstrap(ctx, pool, cfg, quietLogger())
		if !errors.Is(err, user.ErrSeedInProd) {
			t.Fatalf("err = %v, want ErrSeedInProd", err)
		}
	})
	t.Run("prod without seed migrates fine", func(t *testing.T) {
		pool := dbxtest.New(t)
		cfg := user.Config{}
		cfg.Env = "prod"
		if err := user.Bootstrap(ctx, pool, cfg, quietLogger()); err != nil {
			t.Fatal(err)
		}
		if n := count(pool); n != 0 {
			t.Fatalf("users = %d, want 0", n)
		}
	})
}
