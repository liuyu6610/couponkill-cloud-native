package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

func TestLogoutRevokesOnlyTheCurrentJTI(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	r1, _ := login(e, "demo", "demo1234")
	r2, _ := login(e, "demo", "demo1234")
	s1, _ := e.tokens.Verify(r1.Token)
	s2, _ := e.tokens.Verify(r2.Token)

	if err := e.svc.Logout(context.Background(), authx.Identity{UserID: 10000, Roles: []string{"user"}}, r1.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if e.sessions.active(s1.JTI) {
		t.Fatal("当前令牌的会话应被吊销")
	}
	if !e.sessions.active(s2.JTI) {
		t.Fatal("同一用户的另一个会话不得受影响（按令牌吊销，不是按用户）")
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	r, _ := login(e, "demo", "demo1234")
	id := authx.Identity{UserID: 10000, Roles: []string{"user"}}
	for i := 0; i < 3; i++ {
		if err := e.svc.Logout(context.Background(), id, r.Token); err != nil {
			t.Fatalf("Logout #%d: %v", i+1, err)
		}
	}
}

func TestLogoutStaleLoginDoesNotResurrectRevokedToken(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	r1, _ := login(e, "demo", "demo1234")
	s1, _ := e.tokens.Verify(r1.Token)
	id := authx.Identity{UserID: 10000, Roles: []string{"user"}}
	if err := e.svc.Logout(context.Background(), id, r1.Token); err != nil {
		t.Fatal(err)
	}
	login(e, "demo", "demo1234") // 重新登录
	if e.sessions.active(s1.JTI) {
		t.Fatal("重新登录不得让已登出的令牌复活")
	}
}

func TestLogoutRejectsTokenOfAnotherUser(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	r, _ := login(e, "demo", "demo1234")
	s, _ := e.tokens.Verify(r.Token)

	err := e.svc.Logout(context.Background(), authx.Identity{UserID: 777, Roles: []string{"user"}}, r.Token)
	if !errors.Is(err, apperr.TokenInvalid) {
		t.Fatalf("err = %v, want TokenInvalid", err)
	}
	if !e.sessions.active(s.JTI) {
		t.Fatal("身份与令牌不一致时不得吊销任何会话")
	}
}

func TestLogoutRejectsMissingOrGarbageToken(t *testing.T) {
	e := newEnv(t)
	id := authx.Identity{UserID: 10000, Roles: []string{"user"}}
	for _, raw := range []string{"", "not-a-jwt", "a.b.c"} {
		err := e.svc.Logout(context.Background(), id, raw)
		if !errors.Is(err, apperr.TokenInvalid) {
			t.Fatalf("token %q: err = %v, want TokenInvalid", raw, err)
		}
	}
	if len(e.sessions.revoked) != 0 {
		t.Fatalf("不应吊销任何会话: %v", e.sessions.revoked)
	}
}

func TestGetUserReturnsViewWithoutHash(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("demo", "h:demo1234", "user", service.StatusActive, 10000)
	u, err := e.svc.GetUser(context.Background(), 10000)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Username != "demo" || u.ID != 10000 {
		t.Fatalf("unexpected user %+v", u)
	}
}

func TestGetUserNotFoundMapsToUserNotFound(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.GetUser(context.Background(), 424242)
	if !errors.Is(err, apperr.UserNotFound) {
		t.Fatalf("err = %v, want UserNotFound", err)
	}
}

func TestGetUserRepoFailureIsSystemError(t *testing.T) {
	e := newEnv(t)
	e.repo.failNext = errBoom
	_, err := e.svc.GetUser(context.Background(), 1)
	if !errors.Is(err, errBoom) || apperr.Resolve(err).Code != apperr.CodeSysError {
		t.Fatalf("err = %v", err)
	}
}
