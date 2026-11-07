package gateway_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func TestAuth_PublicPathsNeedNoTokenAndCarryNoIdentity(t *testing.T) {
	e := newEnv(t, nil)
	cases := []struct {
		method, path string
		up           *fakeUpstream
	}{
		{"POST", "/api/v1/user/register", e.user},
		{"POST", "/api/v1/user/login", e.user},
		{"GET", "/api/v1/coupon/available", e.coupon},
		{"GET", "/api/v1/coupon/list", e.coupon},
		{"GET", "/api/v1/coupon/1001", e.coupon},
		{"GET", "/api/v1/connector/health", e.connector},
		{"GET", "/api/v1/connector/bindings/by-coupon/1001", e.connector},
		{"GET", "/api/v1/connector/price-compare?couponId=1001", e.connector},
	}
	for _, tc := range cases {
		res := e.do(tc.method, tc.path)
		if res.ResponseRecorder.Code != http.StatusOK {
			t.Errorf("%s %s -> %d, want 200 without a token", tc.method, tc.path, res.ResponseRecorder.Code)
			continue
		}
		last := tc.up.Last(t)
		if last.Header.Get(authx.HeaderUserID) != "" || last.Header.Get(authx.HeaderGatewaySig) != "" {
			t.Errorf("%s %s: public request must carry no identity, got %v", tc.method, tc.path, last.Header)
		}
	}
}

func TestAuth_ProtectedPathsRequireLogin(t *testing.T) {
	e := newEnv(t, nil)
	for _, tc := range [][2]string{
		{"GET", "/api/v1/user/profile"},
		{"POST", "/api/v1/user/logout"},
		{"POST", "/api/v1/order/seckill"},
		{"GET", "/api/v1/order/user/me"},
		{"GET", "/api/v1/user/coupon/count"},
		{"POST", "/api/v1/coupon/create"},
		{"DELETE", "/api/v1/coupon/1001"},
		{"POST", "/api/v1/coupon/1001/status"},
		{"GET", "/api/v1/connector/bindings"},
		{"GET", "/api/v1/connector/platforms"},
		{"GET", "/api/v1/auth/anything"},
		{"GET", "/api/v1/coupon/admin/grant"},
		{"GET", "/api/v1/connector/bindings/by-coupon/not-a-number"},
	} {
		res := e.do(tc[0], tc[1])
		if res.ResponseRecorder.Code != http.StatusUnauthorized || res.Code != apperr.CodeAuthFail {
			t.Errorf("%s %s without token -> %d/%d, want 401/20001", tc[0], tc[1], res.ResponseRecorder.Code, res.Code)
		}
	}
	for n, u := range map[string]*fakeUpstream{"user": e.user, "coupon": e.coupon, "order": e.order, "connector": e.connector} {
		if u.Count() != 0 {
			t.Errorf("upstream %s was reached by %d unauthenticated requests", n, u.Count())
		}
	}
}

func TestAuth_SpoofedIdentityHeadersAreReplacedOnAuthenticatedPaths(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(42, "user")

	e.do("GET", "/api/v1/order/user/me", bearer(token),
		hdr("X-User-Id", "1"), hdr("X-User-Roles", "admin"), hdr("X-Authenticated", "true"),
		hdr("X-Gateway-Ts", "1"), hdr("X-Gateway-Sig", "forged"), hdr("x-gateway-evil", "1"))
	got := e.order.Last(t)
	id := verifyIdentity(t, got.Header, e.clock.Now())
	if id.UserID != 42 || id.IsAdmin() {
		t.Fatalf("upstream saw %+v, want user 42 without admin (client headers must lose)", id)
	}
	if got.Header.Get(authx.HeaderAuthenticated) != "" || got.Header.Get("X-Gateway-Evil") != "" {
		t.Errorf("spoofed X-Authenticated / X-Gateway-* leaked: %v", got.Header)
	}
	if n := len(got.Header.Values(authx.HeaderUserID)); n != 1 {
		t.Errorf("X-User-Id appears %d times, want exactly 1", n)
	}
}

func TestAuth_SpoofedHeadersAreNotAnAuthenticationBypass(t *testing.T) {
	e := newEnv(t, nil)
	signed := authx.SignGateway([]byte(testkit.GatewayKey), 1, []string{"admin"}, e.clock.Now().Unix())
	res := e.do("POST", "/api/v1/coupon/create",
		hdr("X-User-Id", "1"), hdr("X-User-Roles", "admin"), hdr("X-Gateway-Ts", "1"), hdr("X-Gateway-Sig", signed))
	if res.ResponseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: identity headers are never an authentication", res.ResponseRecorder.Code)
	}
	if e.coupon.Count() != 0 {
		t.Fatal("request must not reach the upstream")
	}
}

func TestAuth_RejectsInvalidTokens(t *testing.T) {
	e := newEnv(t, nil)

	wrongSecret := e.newTokens("another-secret-another-secret-0123456789", testIssuer, testAudience)
	wrongAud := e.newTokens(testkit.JWTSecret, testIssuer, "somebody-else")
	wrongIss := e.newTokens(testkit.JWTSecret, "evil-issuer", testAudience)

	forgedRaw, _ := e.loginWith(wrongSecret, 1, "admin")
	wrongAudRaw, _ := e.loginWith(wrongAud, 1, "user")
	wrongIssRaw, _ := e.loginWith(wrongIss, 1, "user")

	noneTok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "1", "iss": testIssuer, "aud": testAudience, "jti": "x",
		"exp": e.clock.Now().Add(time.Hour).Unix(), "roles": []string{"admin"},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	noExp, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "1", "iss": testIssuer, "aud": testAudience, "jti": "noexp", "roles": []string{"user"},
	}).SignedString([]byte(testkit.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.sessions.Open(context.Background(), "noexp", time.Hour); err != nil {
		t.Fatal(err)
	}
	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub": "1", "iss": testIssuer, "aud": testAudience, "jti": "hs512",
		"exp": e.clock.Now().Add(time.Hour).Unix(), "roles": []string{"user"},
	}).SignedString([]byte(testkit.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.sessions.Open(context.Background(), "hs512", time.Hour); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"forged signature": forgedRaw,
		"wrong audience":   wrongAudRaw,
		"wrong issuer":     wrongIssRaw,
		"alg none":         noneTok,
		"missing exp":      noExp,
		"alg HS512":        hs512,
		"garbage":          "not.a.jwt",
		"truncated":        forgedRaw[:len(forgedRaw)-5],
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			res := e.do("GET", "/api/v1/order/user/me", bearer(raw))
			if res.ResponseRecorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (%s)", res.ResponseRecorder.Code, res.Body.String())
			}
			if res.Code != apperr.CodeTokenInvalid {
				t.Errorf("code = %d, want %d", res.Code, apperr.CodeTokenInvalid)
			}
			if e.order.Count() != 0 {
				t.Fatal("rejected token must not reach the upstream")
			}
		})
	}
}

func TestAuth_ExpiredTokenIs401AuthFailAfterLeeway(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(42, "user") // TTL 1h

	e.clock.Advance(time.Hour + 20*time.Second) // 在 30s 宽限内仍有效
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Fatalf("within leeway: status = %d, want 200", res.ResponseRecorder.Code)
	}
	e.clock.Advance(15 * time.Second) // 总计超过 TTL+30s
	res := e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != http.StatusUnauthorized || res.Code != apperr.CodeAuthFail {
		t.Fatalf("expired: status/code = %d/%d, want 401/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeAuthFail)
	}
}

func TestAuth_MalformedAuthorizationHeaders(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(42, "user")
	for name, v := range map[string]string{
		"no scheme":     token,
		"basic scheme":  "Basic " + token,
		"empty bearer":  "Bearer ",
		"bearer only":   "Bearer",
		"double scheme": "Bearer Bearer " + token,
	} {
		res := e.do("GET", "/api/v1/order/user/me", hdr("Authorization", v))
		if res.ResponseRecorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, res.ResponseRecorder.Code)
		}
	}
	// scheme 不区分大小写；令牌不接受放在查询参数里。
	if res := e.do("GET", "/api/v1/order/user/me", hdr("Authorization", "bearer "+token)); res.ResponseRecorder.Code != 200 {
		t.Errorf("lower-case scheme: status = %d, want 200", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/order/user/me?token="+token); res.ResponseRecorder.Code != http.StatusUnauthorized {
		t.Errorf("token in query string must not authenticate, status = %d", res.ResponseRecorder.Code)
	}
}

func TestAuth_RevokedSessionIs401EvenWithValidSignature(t *testing.T) {
	e := newEnv(t, nil)
	token, jti := e.login(42, "user")
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Fatalf("before logout: status = %d", res.ResponseRecorder.Code)
	}

	if err := e.sessions.Revoke(context.Background(), jti); err != nil {
		t.Fatal(err)
	}
	res := e.do("GET", "/api/v1/order/user/me", bearer(token))
	if res.ResponseRecorder.Code != http.StatusUnauthorized || res.Code != apperr.CodeAuthFail {
		t.Fatalf("after logout: status/code = %d/%d, want 401/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeAuthFail)
	}

	// 重新登录是另一个 jti：旧令牌不会复活，新令牌可用。
	newToken, _ := e.login(42, "user")
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 401 {
		t.Errorf("old token revived after re-login: %d", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/order/user/me", bearer(newToken)); res.ResponseRecorder.Code != 200 {
		t.Errorf("new token rejected: %d", res.ResponseRecorder.Code)
	}
}

func TestAuth_SessionExpiryInRedisInvalidatesToken(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(42, "user")
	e.mr.FastForward(2 * time.Hour) // 会话键 TTL 已过，令牌本身（假时钟未动）仍未过期
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 once the session key is gone", res.ResponseRecorder.Code)
	}
}

func TestAuth_RevocationDisabledSkipsRedisEntirely(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_REVOCATION_ENABLED": "false"})
	token, jti := e.login(42, "user")
	if err := e.sessions.Revoke(context.Background(), jti); err != nil {
		t.Fatal(err)
	}
	e.mr.Close() // Redis 完全不可用也不影响
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Fatalf("status = %d, want 200: with revocation disabled only the JWT is checked", res.ResponseRecorder.Code)
	}
}

func TestAuth_RedisOutageFailOpenVersusFailClosed(t *testing.T) {
	t.Run("fail-open (default) lets the request through", func(t *testing.T) {
		e := newEnv(t, nil)
		token, _ := e.login(42, "user")
		e.mr.Close()
		res := e.do("GET", "/api/v1/order/user/me", bearer(token))
		if res.ResponseRecorder.Code != 200 {
			t.Fatalf("status = %d, want 200 when failing open", res.ResponseRecorder.Code)
		}
		verifyIdentity(t, e.order.Last(t).Header, e.clock.Now())
	})
	t.Run("fail-closed answers 503", func(t *testing.T) {
		e := newEnv(t, map[string]string{"CK_GATEWAY_REVOCATION_FAIL_OPEN": "false"})
		token, _ := e.login(42, "user")
		e.mr.Close()
		res := e.do("GET", "/api/v1/order/user/me", bearer(token))
		if res.ResponseRecorder.Code != http.StatusServiceUnavailable || res.Code != apperr.CodeSystemBusy {
			t.Fatalf("status/code = %d/%d, want 503/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeSystemBusy)
		}
		if e.order.Count() != 0 {
			t.Fatal("fail-closed must not reach the upstream")
		}
	})
	t.Run("an invalid token is still rejected while failing open", func(t *testing.T) {
		e := newEnv(t, nil)
		e.mr.Close()
		res := e.do("GET", "/api/v1/order/user/me", bearer("not.a.jwt"))
		if res.ResponseRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.ResponseRecorder.Code)
		}
	})
}

// blockingRevoker 模拟响应极慢的会话存储：一直阻塞到调用方的 ctx 到期。
type blockingRevoker struct{}

func (blockingRevoker) Open(context.Context, string, time.Duration) error { return nil }
func (blockingRevoker) Revoke(context.Context, string) error              { return nil }
func (blockingRevoker) Active(ctx context.Context, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func TestAuth_SlowSessionStoreHitsTheRedisTimeout(t *testing.T) {
	t.Run("fail-open", func(t *testing.T) {
		e := newEnv(t, map[string]string{"CK_GATEWAY_REDIS_TIMEOUT": "30ms"}, withRevoker(blockingRevoker{}))
		token, _ := e.login(42, "user")
		start := time.Now()
		res := e.do("GET", "/api/v1/order/user/me", bearer(token))
		if res.ResponseRecorder.Code != 200 {
			t.Fatalf("status = %d, want 200", res.ResponseRecorder.Code)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("request took %v: the session check must give up after the redis timeout", d)
		}
	})
	t.Run("fail-closed", func(t *testing.T) {
		e := newEnv(t, map[string]string{"CK_GATEWAY_REDIS_TIMEOUT": "30ms", "CK_GATEWAY_REVOCATION_FAIL_OPEN": "false"}, withRevoker(blockingRevoker{}))
		token, _ := e.login(42, "user")
		start := time.Now()
		res := e.do("GET", "/api/v1/order/user/me", bearer(token))
		if res.ResponseRecorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", res.ResponseRecorder.Code)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("request took %v", d)
		}
	})
}

func TestAuth_SignedHeadersVerifyAndExpireAfterTheWindow(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(777, "user", "admin")
	e.do("GET", "/api/v1/order/user/me", bearer(token))
	h := e.order.Last(t).Header

	signer, err := authx.NewGatewaySigner(testkit.GatewayKey, authx.WithSignerClock(e.clock.Now))
	if err != nil {
		t.Fatal(err)
	}
	id, err := signer.Verify(h)
	if err != nil || id.UserID != 777 || !id.IsAdmin() {
		t.Fatalf("Verify = %+v, %v", id, err)
	}

	e.clock.Advance(authx.GatewayWindow - time.Second)
	if _, err := signer.Verify(h); err != nil {
		t.Errorf("still inside the window but Verify failed: %v", err)
	}
	e.clock.Advance(2 * time.Second)
	if _, err := signer.Verify(h); err == nil {
		t.Error("headers must expire once the window has passed")
	}

	// 篡改任何一个字段都应验签失败。
	tampered := h.Clone()
	tampered.Set(authx.HeaderUserID, "1")
	e.clock.Advance(-time.Minute)
	if _, err := signer.Verify(tampered); err == nil {
		t.Error("tampered X-User-Id must not verify")
	}

	// 下游用错误的密钥验签失败。
	other, _ := authx.NewGatewaySigner(strings.Repeat("k", 40), authx.WithSignerClock(e.clock.Now))
	if _, err := other.Verify(h); err == nil {
		t.Error("a different signing key must not verify")
	}
}

// 跨包约定：user 服务登出时要从原始 Authorization 头里解析 jti，并核对 sub 与网关确认的 X-User-Id 一致，
// 所以网关必须原样转发 Authorization，身份头由网关重新签名，客户端自带的 X-User-Id 被剥掉。
func TestAuth_LogoutReachesUserServiceWithOriginalTokenAndResignedIdentity(t *testing.T) {
	e := newEnv(t, nil)
	token, jti := e.login(42, "user")

	res := e.do("POST", "/api/v1/user/logout", bearer(token), hdr("X-User-Id", "1"), hdr("X-User-Roles", "admin"))
	if res.ResponseRecorder.Code != 200 {
		t.Fatalf("status = %d: %s", res.ResponseRecorder.Code, res.Body.String())
	}
	got := e.user.Last(t)
	if got.Path != "/api/v1/user/logout" || got.Method != "POST" {
		t.Fatalf("user upstream got %s %s", got.Method, got.Path)
	}
	if got.Header.Get("Authorization") != "Bearer "+token {
		t.Fatalf("Authorization = %q, want the original value forwarded unchanged", got.Header.Get("Authorization"))
	}
	id := verifyIdentity(t, got.Header, e.clock.Now())
	if id.UserID != 42 || id.IsAdmin() {
		t.Fatalf("identity = %+v, want gateway-signed user 42 (client X-User-Id/X-User-Roles stripped)", id)
	}
	// user 服务能用同一个令牌解析出 jti，且 sub 与网关确认的身份一致。
	sess, err := e.tokens.Verify(strings.TrimPrefix(got.Header.Get("Authorization"), "Bearer "))
	if err != nil || sess.JTI != jti || sess.Identity.UserID != id.UserID {
		t.Fatalf("forwarded token resolves to %+v, %v; want jti %s and sub %d", sess, err, jti, id.UserID)
	}
}

func TestAuth_EachRequestGetsFreshTimestamp(t *testing.T) {
	e := newEnv(t, nil)
	token, _ := e.login(1, "user")
	e.do("GET", "/api/v1/order/user/me", bearer(token))
	first := e.order.Last(t).Header.Get(authx.HeaderGatewayTS)
	e.clock.Advance(10 * time.Second)
	e.do("GET", "/api/v1/order/user/me", bearer(token))
	second := e.order.Last(t).Header.Get(authx.HeaderGatewayTS)
	if first == second {
		t.Errorf("timestamps equal (%s): each forwarded request must be signed anew", first)
	}
}
