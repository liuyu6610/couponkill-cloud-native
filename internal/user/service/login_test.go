package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

func login(e *env, username, password string) (service.LoginResult, error) {
	return e.svc.Login(context.Background(), service.LoginInput{Username: username, Password: password})
}

func TestLoginSuccessIssuesTokenOpensSessionAndReturnsRoles(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)

	res, err := login(e, "demo", "demo1234")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.User.ID != 10000 || res.User.Username != "demo" {
		t.Fatalf("unexpected user %+v", res.User)
	}
	if !reflect.DeepEqual(res.Roles, []string{"user"}) {
		t.Fatalf("roles = %v, want [user]", res.Roles)
	}
	sess, err := e.tokens.Verify(res.Token)
	if err != nil {
		t.Fatalf("issued token must verify: %v", err)
	}
	if sess.Identity.UserID != 10000 || !reflect.DeepEqual(sess.Identity.Roles, []string{"user"}) {
		t.Fatalf("token identity = %+v", sess.Identity)
	}
	if !res.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Fatalf("ExpiresAt %v != token exp %v", res.ExpiresAt, sess.ExpiresAt)
	}
	if !e.sessions.active(sess.JTI) {
		t.Fatal("登录必须登记服务端会话，网关才认这个令牌")
	}
	if ttl := e.sessions.open[sess.JTI]; ttl != time.Hour {
		t.Fatalf("会话 TTL = %v, want 令牌剩余寿命 1h", ttl)
	}
}

func TestLoginAdminGetsAdminAndUserRoles(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("admin", "h:admin1234", "admin", service.StatusActive, 10001)
	res, err := login(e, "admin", "admin1234")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !reflect.DeepEqual(res.Roles, []string{"admin", "user"}) {
		t.Fatalf("roles = %v, want [admin user]（沿用 Java 语义）", res.Roles)
	}
	sess, err := e.tokens.Verify(res.Token)
	if err != nil || !sess.Identity.IsAdmin() {
		t.Fatalf("token should carry admin role: %+v %v", sess.Identity, err)
	}
}

func TestLoginUnknownUserAndWrongPasswordAreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)

	_, errWrong := login(e, "demo", "bad-password")
	_, errUnknown := login(e, "nobody", "bad-password")

	for name, err := range map[string]error{"wrong password": errWrong, "unknown user": errUnknown} {
		if !errors.Is(err, apperr.PasswordError) {
			t.Fatalf("%s: err = %v, want PasswordError", name, err)
		}
	}
	a, b := apperr.Resolve(errWrong), apperr.Resolve(errUnknown)
	if a.Code != b.Code || a.HTTPStatus != b.HTTPStatus || a.Message != b.Message {
		t.Fatalf("响应必须一致: wrong=%+v unknown=%+v", a, b)
	}
	if a.Message != "用户名或密码错误" || a.HTTPStatus != 401 {
		t.Fatalf("message/status = %q/%d", a.Message, a.HTTPStatus)
	}
}

func TestLoginDoesEquivalentHashWorkForUnknownAndKnownUsers(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)

	login(e, "demo", "bad")
	if v, d := e.hasher.verifyCalls.Load(), e.hasher.dummyCalls.Load(); v != 1 || d != 0 {
		t.Fatalf("已知用户错误口令: verify=%d dummy=%d, want 1/0", v, d)
	}
	login(e, "nobody", "bad")
	if v, d := e.hasher.verifyCalls.Load(), e.hasher.dummyCalls.Load(); v != 2 || d != 1 {
		t.Fatalf("未知用户必须也走一次 VerifyOrDummy: verify=%d dummy=%d, want 2/1", v, d)
	}
	login(e, "demo", "demo1234")
	if v := e.hasher.verifyCalls.Load(); v != 3 {
		t.Fatalf("成功登录同样只做一次校验: verify=%d, want 3", v)
	}
	if h := e.hasher.hashCalls.Load(); h != 0 {
		t.Fatalf("登录不应生成新哈希: %d", h)
	}
}

func TestLoginRejectsEmptyCredentialsBeforeAnyLookup(t *testing.T) {
	e := newEnv(t)
	for _, in := range []service.LoginInput{{"", "x"}, {"demo", ""}, {"  ", "x"}, {"", ""}} {
		_, err := e.svc.Login(context.Background(), in)
		if !errors.Is(err, apperr.InvalidRequest) {
			t.Fatalf("%+v: err = %v, want InvalidRequest", in, err)
		}
	}
	if _, finds, _ := e.repo.counts(); finds != 0 {
		t.Fatalf("参数不合法不应查库，FindByUsername 被调用 %d 次", finds)
	}
}

func TestLoginOverlongPasswordIsPlainFailureNotCrash(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	long := make([]byte, 2000)
	for i := range long {
		long[i] = 'x'
	}
	_, err := login(e, "demo", string(long))
	if !errors.Is(err, apperr.PasswordError) && !errors.Is(err, apperr.InvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginDisabledAccountCannotLogInEvenWithRightPassword(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("banned", "h:secret1", "user", service.StatusDisabled, 5)
	_, err := login(e, "banned", "secret1")
	if !errors.Is(err, apperr.Forbidden) {
		t.Fatalf("err = %v, want Forbidden", err)
	}
	if len(e.sessions.open) != 0 {
		t.Fatal("停用账号不得产生会话")
	}
	// 口令错误时不泄露“已停用”
	_, err = login(e, "banned", "wrong")
	if !errors.Is(err, apperr.PasswordError) {
		t.Fatalf("wrong password on disabled account: err = %v, want PasswordError", err)
	}
}

func TestLoginRepoFailureIsSystemError(t *testing.T) {
	e := newEnv(t)
	e.repo.failNext = errBoom
	_, err := login(e, "demo", "demo1234")
	if errors.Is(err, apperr.PasswordError) {
		t.Fatalf("基础设施故障不能伪装成口令错误: %v", err)
	}
	if !errors.Is(err, errBoom) || apperr.Resolve(err).Code != apperr.CodeSysError {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginSessionOpenFailureDoesNotHandOutToken(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	e.sessions.openErr = errBoom
	res, err := login(e, "demo", "demo1234")
	if err == nil {
		t.Fatal("会话登记失败时登录必须失败")
	}
	if res.Token != "" {
		t.Fatal("失败时不得返回令牌")
	}
	if got := apperr.Resolve(err); got.Code != apperr.CodeSystemBusy {
		t.Fatalf("code = %d, want SystemBusy", got.Code)
	}
}

func TestLoginTokensAreIndependentSessions(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	r1, _ := login(e, "demo", "demo1234")
	r2, _ := login(e, "demo", "demo1234")
	s1, _ := e.tokens.Verify(r1.Token)
	s2, _ := e.tokens.Verify(r2.Token)
	if s1.JTI == s2.JTI {
		t.Fatal("每次登录必须有不同的 jti")
	}
	if !e.sessions.active(s1.JTI) || !e.sessions.active(s2.JTI) {
		t.Fatal("两个会话应同时有效")
	}
}

func TestLoginRefreshesLastActiveAtMostOncePerHour(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)

	login(e, "demo", "demo1234")
	if _, _, touches := e.repo.counts(); touches != 1 {
		t.Fatalf("首次登录应刷新一次, touches=%d", touches)
	}
	e.clock.Advance(30 * time.Minute)
	login(e, "demo", "demo1234")
	if _, _, touches := e.repo.counts(); touches != 1 {
		t.Fatalf("1 小时内再次登录不应再写库, touches=%d", touches)
	}
	e.clock.Advance(31 * time.Minute)
	login(e, "demo", "demo1234")
	if _, _, touches := e.repo.counts(); touches != 2 {
		t.Fatalf("超过 1 小时应再刷新一次, touches=%d", touches)
	}
}

func TestLoginFailedAttemptsNeverTouchLastActive(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	login(e, "demo", "wrong")
	login(e, "nobody", "x")
	if _, _, touches := e.repo.counts(); touches != 0 {
		t.Fatalf("失败登录不得刷新活跃时间, touches=%d", touches)
	}
}

func TestLoginSucceedsEvenIfLastActiveRefreshFails(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	e.repo.touchErr = errBoom
	if _, err := login(e, "demo", "demo1234"); err != nil {
		t.Fatalf("活跃时间刷新失败不应阻断登录: %v", err)
	}
}
