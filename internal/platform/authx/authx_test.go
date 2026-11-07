package authx_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
)

const secret = "0123456789abcdef0123456789abcdef-jwt"

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)} }
func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func tokens(t *testing.T, clk *clock, mut ...func(*authx.TokenConfig)) *authx.Tokens {
	t.Helper()
	cfg := authx.TokenConfig{Secret: secret, Issuer: "couponkill", Audience: "couponkill-api", TTL: time.Hour, Now: clk.Now}
	for _, m := range mut {
		m(&cfg)
	}
	tk, err := authx.NewTokens(cfg)
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	return tk
}

// ---------- JWT ----------

func TestNewTokensRejectsShortSecretAndMissingClaimsConfig(t *testing.T) {
	if _, err := authx.NewTokens(authx.TokenConfig{Secret: "short", Issuer: "i", Audience: "a"}); err == nil {
		t.Error("secret shorter than 32 bytes must be rejected")
	}
	if _, err := authx.NewTokens(authx.TokenConfig{Secret: secret, Audience: "a"}); err == nil {
		t.Error("empty issuer must be rejected")
	}
	if _, err := authx.NewTokens(authx.TokenConfig{Secret: secret, Issuer: "i"}); err == nil {
		t.Error("empty audience must be rejected")
	}
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	clk := newClock()
	tk := tokens(t, clk)
	tok, err := tk.Issue(authx.Identity{UserID: 9007199254740993, Roles: []string{"user", "admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if tok.JTI == "" || len(tok.JTI) < 16 || !tok.ExpiresAt.Equal(clk.Now().Add(time.Hour)) {
		t.Fatalf("token metadata wrong: %+v", tok)
	}
	sess, err := tk.Verify(tok.Raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if sess.Identity.UserID != 9007199254740993 || !sess.Identity.IsAdmin() || sess.JTI != tok.JTI || !sess.ExpiresAt.Equal(tok.ExpiresAt) {
		t.Fatalf("session mismatch: %+v", sess)
	}
}

func TestEachTokenGetsUniqueJTI(t *testing.T) {
	tk := tokens(t, newClock())
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok, err := tk.Issue(authx.Identity{UserID: 1})
		if err != nil || seen[tok.JTI] {
			t.Fatalf("jti reuse or error: %v", err)
		}
		seen[tok.JTI] = true
	}
}

func TestIssueRejectsInvalidIdentity(t *testing.T) {
	tk := tokens(t, newClock())
	if _, err := tk.Issue(authx.Identity{UserID: 0}); err == nil {
		t.Error("user id 0 must be rejected")
	}
	if _, err := tk.Issue(authx.Identity{UserID: 1, Roles: []string{"a,b"}}); err == nil {
		t.Error("role containing a comma must be rejected")
	}
}

func TestVerifyExpiryAndLeeway(t *testing.T) {
	clk := newClock()
	tk := tokens(t, clk)
	tok, _ := tk.Issue(authx.Identity{UserID: 1})
	clk.Add(time.Hour + 29*time.Second) // 在 30s 宽限内
	if _, err := tk.Verify(tok.Raw); err != nil {
		t.Fatalf("expired within leeway must still verify: %v", err)
	}
	clk.Add(2 * time.Second) // 超过宽限
	_, err := tk.Verify(tok.Raw)
	if !errors.Is(err, apperr.AuthFail) {
		t.Fatalf("expired token must map to AuthFail, got %v", err)
	}
	if apperr.StatusOf(err) != http.StatusUnauthorized {
		t.Fatalf("status = %d", apperr.StatusOf(err))
	}
}

func TestVerifyRejectsForgeries(t *testing.T) {
	clk := newClock()
	tk := tokens(t, clk)
	good, _ := tk.Issue(authx.Identity{UserID: 5, Roles: []string{"user"}})

	claims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"sub": "5", "roles": []string{"admin"}, "iss": "couponkill", "aud": "couponkill-api",
			"jti": "abc", "exp": clk.Now().Add(time.Hour).Unix(), "iat": clk.Now().Unix(),
		}
	}
	sign := func(m jwt.SigningMethod, key any, c jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	withClaim := func(k string, v any) jwt.MapClaims { c := claims(); c[k] = v; return c }
	without := func(k string) jwt.MapClaims { c := claims(); delete(c, k); return c }

	parts := strings.Split(good.Raw, ".")
	tamperedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"1","roles":["admin"],"iss":"couponkill","aud":"couponkill-api","jti":"x","exp":9999999999}`))

	cases := map[string]string{
		"alg none":         sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims()),
		"HS512 not pinned": sign(jwt.SigningMethodHS512, []byte(secret), claims()),
		"wrong secret":     sign(jwt.SigningMethodHS256, []byte("another-secret-another-secret-123456"), claims()),
		"wrong issuer":     sign(jwt.SigningMethodHS256, []byte(secret), withClaim("iss", "evil")),
		"wrong audience":   sign(jwt.SigningMethodHS256, []byte(secret), withClaim("aud", "other")),
		"missing exp":      sign(jwt.SigningMethodHS256, []byte(secret), without("exp")),
		"missing jti":      sign(jwt.SigningMethodHS256, []byte(secret), without("jti")),
		"non-numeric sub":  sign(jwt.SigningMethodHS256, []byte(secret), withClaim("sub", "abc")),
		"zero sub":         sign(jwt.SigningMethodHS256, []byte(secret), withClaim("sub", "0")),
		"missing sub":      sign(jwt.SigningMethodHS256, []byte(secret), without("sub")),
		"tampered payload": parts[0] + "." + tamperedPayload + "." + parts[2],
		"garbage":          "not-a-jwt",
		"empty":            "",
		"future iat":       sign(jwt.SigningMethodHS256, []byte(secret), withClaim("iat", clk.Now().Add(time.Hour).Unix())),
		"truncated sig":    good.Raw[:len(good.Raw)-4],
	}
	for name, raw := range cases {
		_, err := tk.Verify(raw)
		if err == nil {
			t.Errorf("%s: forged token accepted", name)
			continue
		}
		if !errors.Is(err, apperr.TokenInvalid) {
			t.Errorf("%s: want TokenInvalid, got %v", name, err)
		}
	}
	// 对照：同样的构造方式、合法的 claims 是能通过的，说明上面的失败来自被篡改的那一项。
	if _, err := tk.Verify(sign(jwt.SigningMethodHS256, []byte(secret), claims())); err != nil {
		t.Fatalf("control token must verify: %v", err)
	}
}

func TestVerifyAcceptsAudienceListContainingExpected(t *testing.T) {
	clk := newClock()
	tk := tokens(t, clk)
	c := jwt.MapClaims{"sub": "5", "iss": "couponkill", "aud": []string{"x", "couponkill-api"}, "jti": "j", "exp": clk.Now().Add(time.Hour).Unix()}
	raw, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	if _, err := tk.Verify(raw); err != nil {
		t.Fatalf("aud list containing expected audience must pass: %v", err)
	}
}

// ---------- 口令 ----------

func TestPasswordHashing(t *testing.T) {
	h, err := authx.HashPassword("demo1234")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$2a$10$") && !strings.HasPrefix(h, "$2b$10$") {
		t.Fatalf("expected bcrypt cost 10, got prefix %q", h[:7])
	}
	h2, _ := authx.HashPassword("demo1234")
	if h == h2 {
		t.Error("hashes must be salted")
	}
	if !authx.VerifyPassword(h, "demo1234") || authx.VerifyPassword(h, "demo12345") || authx.VerifyPassword("", "demo1234") || authx.VerifyPassword("garbage", "x") {
		t.Error("VerifyPassword semantics wrong")
	}
	if _, err := authx.HashPassword(strings.Repeat("x", 73)); err == nil {
		t.Error("passwords over bcrypt's 72-byte limit must be rejected, not silently truncated")
	}
}

func TestVerifyOrDummyDoesConstantWork(t *testing.T) {
	h, _ := authx.HashPassword("pw")
	if !authx.VerifyOrDummy(h, "pw") || authx.VerifyOrDummy(h, "bad") {
		t.Fatal("known user path wrong")
	}
	authx.VerifyOrDummy("", "warm-up") // 初始化哑哈希不计入耗时
	start := time.Now()
	if authx.VerifyOrDummy("", "whatever") {
		t.Fatal("unknown user must never verify")
	}
	if d := time.Since(start); d < 5*time.Millisecond {
		t.Fatalf("dummy comparison took %v: unknown users would be distinguishable by timing", d)
	}
}

// ---------- 网关身份 ----------

const signKey = "gateway-sign-key-gateway-sign-key-0123"

func signer(t *testing.T, clk *clock) *authx.GatewaySigner {
	t.Helper()
	gs, err := authx.NewGatewaySigner(signKey, authx.WithSignerClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	return gs
}

func TestNewGatewaySignerRejectsShortKey(t *testing.T) {
	if _, err := authx.NewGatewaySigner("short"); err == nil {
		t.Fatal("short key must be rejected")
	}
}

func TestGatewayApplyVerifyRoundTrip(t *testing.T) {
	clk := newClock()
	gs := signer(t, clk)
	h := http.Header{}
	// 客户端伪造的头必须被覆盖/清除。
	h.Set("X-User-Id", "1")
	h.Set("X-Authenticated", "true")
	h.Set("X-Gateway-Extra", "spoof")
	if err := gs.Apply(h, authx.Identity{UserID: 42, Roles: []string{"user", "admin"}}); err != nil {
		t.Fatal(err)
	}
	if h.Get("X-Authenticated") != "" || h.Get("X-Gateway-Extra") != "" {
		t.Fatalf("Apply must strip foreign identity headers: %v", h)
	}
	if h.Get("X-User-Id") != "42" || h.Get("X-User-Roles") != "user,admin" || h.Get("X-Gateway-Ts") == "" || h.Get("X-Gateway-Sig") == "" {
		t.Fatalf("headers incomplete: %v", h)
	}
	id, err := gs.Verify(h)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.UserID != 42 || !id.IsAdmin() || len(id.Roles) != 2 {
		t.Fatalf("identity mismatch: %+v", id)
	}
}

func TestGatewaySignatureFormat(t *testing.T) {
	// 钉死 design §5 的规格：HMAC-SHA256(key, "{userId}|{roles}|{ts}") 的十六进制。
	got := authx.SignGateway([]byte(signKey), 42, []string{"user", "admin"}, 1780315200)
	if len(got) != 64 {
		t.Fatalf("want 64 hex chars, got %q", got)
	}
	if got != authx.SignGateway([]byte(signKey), 42, []string{"user", "admin"}, 1780315200) {
		t.Fatal("signature must be deterministic")
	}
	if got == authx.SignGateway([]byte(signKey), 42, []string{"admin", "user"}, 1780315200) {
		t.Fatal("roles order is part of the signed payload")
	}
}

func TestGatewayVerifyRejectsTampering(t *testing.T) {
	clk := newClock()
	gs := signer(t, clk)
	fresh := func() http.Header {
		h := http.Header{}
		if err := gs.Apply(h, authx.Identity{UserID: 42, Roles: []string{"user"}}); err != nil {
			t.Fatal(err)
		}
		return h
	}
	cases := map[string]func(http.Header){
		"changed user id":   func(h http.Header) { h.Set("X-User-Id", "43") },
		"escalated roles":   func(h http.Header) { h.Set("X-User-Roles", "user,admin") },
		"changed timestamp": func(h http.Header) { h.Set("X-Gateway-Ts", "1") },
		"flipped signature": func(h http.Header) { h.Set("X-Gateway-Sig", strings.Repeat("0", 64)) },
		"non-hex signature": func(h http.Header) { h.Set("X-Gateway-Sig", "zz") },
		"missing signature": func(h http.Header) { h.Del("X-Gateway-Sig") },
		"missing user":      func(h http.Header) { h.Del("X-User-Id") },
		"missing timestamp": func(h http.Header) { h.Del("X-Gateway-Ts") },
		"non-canonical id":  func(h http.Header) { h.Set("X-User-Id", "+42") },
		"non-numeric ts":    func(h http.Header) { h.Set("X-Gateway-Ts", "abc") },
		"zero user":         func(h http.Header) { h.Set("X-User-Id", "0") },
		"signature of other": func(h http.Header) {
			h.Set("X-Gateway-Sig", authx.SignGateway([]byte("another-key-another-key-another-key"), 42, []string{"user"}, 0))
		},
	}
	for name, mutate := range cases {
		h := fresh()
		mutate(h)
		if _, err := gs.Verify(h); !errors.Is(err, apperr.AuthFail) {
			t.Errorf("%s: want AuthFail, got %v", name, err)
		}
	}
	if _, err := gs.Verify(fresh()); err != nil {
		t.Fatalf("control must verify: %v", err)
	}
}

func TestGatewayVerifyTimeWindow(t *testing.T) {
	clk := newClock()
	gs := signer(t, clk)
	h := http.Header{}
	if err := gs.Apply(h, authx.Identity{UserID: 7}); err != nil {
		t.Fatal(err)
	}
	clk.Add(60 * time.Second)
	if _, err := gs.Verify(h); err != nil {
		t.Fatalf("exactly 60s old must pass: %v", err)
	}
	clk.Add(1 * time.Second)
	_, err := gs.Verify(h)
	if !errors.Is(err, authx.ErrStaleTimestamp) {
		t.Fatalf("61s old must be stale, got %v", err)
	}
	// 未来时间戳同样受 ±60s 约束（重放窗口对称）。
	clk2 := newClock()
	gsFuture := signer(t, clk2)
	h2 := http.Header{}
	_ = gsFuture.Apply(h2, authx.Identity{UserID: 7})
	clk2.Add(-61 * time.Second)
	if _, err := gsFuture.Verify(h2); !errors.Is(err, authx.ErrStaleTimestamp) {
		t.Fatalf("a timestamp 61s in the future must be rejected, got %v", err)
	}
}

func TestGatewayApplyRejectsUnsignableIdentity(t *testing.T) {
	gs := signer(t, newClock())
	for _, id := range []authx.Identity{{UserID: 0}, {UserID: 1, Roles: []string{"a|b"}}, {UserID: 1, Roles: []string{"a,b"}}, {UserID: 1, Roles: []string{""}}} {
		if err := gs.Apply(http.Header{}, id); err == nil {
			t.Errorf("Apply(%+v) must fail", id)
		}
	}
}

func TestStripIdentityHeaders(t *testing.T) {
	h := http.Header{}
	for _, k := range []string{"X-User-Id", "x-user-roles", "X-Authenticated", "X-Gateway-Ts", "X-Gateway-Sig", "X-Gateway-Anything", "X-Request-Id", "Authorization"} {
		h.Set(k, "v")
	}
	authx.StripIdentityHeaders(h)
	if len(h) != 2 || h.Get("X-Request-Id") == "" || h.Get("Authorization") == "" {
		t.Fatalf("only identity headers may be removed, left: %v", h)
	}
}

// ---------- Identity / ctx / Revoker ----------

func TestIdentityHelpersAndContext(t *testing.T) {
	id := authx.Identity{UserID: 3, Roles: []string{"user"}}
	if id.IsAdmin() || !id.HasRole("user") {
		t.Fatal("role helpers wrong")
	}
	if !(authx.Identity{Roles: []string{"admin"}}).IsAdmin() {
		t.Fatal("admin role not detected")
	}
	if _, ok := authx.IdentityFrom(context.Background()); ok {
		t.Fatal("bare ctx has no identity")
	}
	ctx := authx.WithIdentity(context.Background(), id)
	got, ok := authx.IdentityFrom(ctx)
	if !ok || got.UserID != 3 {
		t.Fatalf("IdentityFrom = %+v, %v", got, ok)
	}
}

func TestMemoryRevokerSemantics(t *testing.T) {
	clk := newClock()
	r := authx.NewMemoryRevoker()
	r.SetClock(clk.Now)
	ctx := context.Background()

	if ok, _ := r.Active(ctx, "j1"); ok {
		t.Fatal("unknown jti must not be active")
	}
	_ = r.Open(ctx, "j1", time.Minute)
	_ = r.Open(ctx, "j2", time.Minute)
	if ok, _ := r.Active(ctx, "j1"); !ok {
		t.Fatal("opened jti must be active")
	}
	if err := r.Revoke(ctx, "j1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.Active(ctx, "j1"); ok {
		t.Fatal("revoked jti must be inactive")
	}
	if ok, _ := r.Active(ctx, "j2"); !ok {
		t.Fatal("revoking one jti must not affect another (per-token revocation)")
	}
	clk.Add(2 * time.Minute)
	if ok, _ := r.Active(ctx, "j2"); ok {
		t.Fatal("jti must expire after its ttl")
	}
	if err := r.Revoke(ctx, "never-opened"); err != nil {
		t.Fatalf("revoking an unknown jti is a no-op, got %v", err)
	}
}
