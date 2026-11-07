package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

func TestRegisterValidationRules(t *testing.T) {
	long50 := strings.Repeat("a", 50)
	tests := []struct {
		name    string
		in      service.RegisterInput
		wantErr string // 空表示应成功；否则为 InvalidRequest 文案的子串
	}{
		{"ok", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "13800000000"}, ""},
		{"username 50 chars ok", service.RegisterInput{Username: long50, Password: "secret1", Phone: "13800000000"}, ""},
		{"unicode username ok", service.RegisterInput{Username: "小明", Password: "secret1", Phone: "13800000000"}, ""},
		{"password exactly 6 ok", service.RegisterInput{Username: "bob", Password: "abcdef", Phone: "19999999999"}, ""},
		{"password 72 bytes ok", service.RegisterInput{Username: "carl", Password: strings.Repeat("p", 72), Phone: "13800000000"}, ""},

		{"empty username", service.RegisterInput{Username: "", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"blank username", service.RegisterInput{Username: "   ", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"username 51 chars", service.RegisterInput{Username: long50 + "a", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"username with space inside", service.RegisterInput{Username: "al ice", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"username with surrounding space", service.RegisterInput{Username: " alice", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"username with tab", service.RegisterInput{Username: "al\tice", Password: "secret1", Phone: "13800000000"}, "用户名"},
		{"username with control char", service.RegisterInput{Username: "al\x00ice", Password: "secret1", Phone: "13800000000"}, "用户名"},

		{"empty password", service.RegisterInput{Username: "alice", Password: "", Phone: "13800000000"}, "密码"},
		{"password 5 chars", service.RegisterInput{Username: "alice", Password: "abcde", Phone: "13800000000"}, "密码"},
		{"password 73 bytes (bcrypt limit)", service.RegisterInput{Username: "alice", Password: strings.Repeat("p", 73), Phone: "13800000000"}, "密码"},

		{"empty phone", service.RegisterInput{Username: "alice", Password: "secret1", Phone: ""}, "手机号"},
		{"phone starts with 12", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "12800000000"}, "手机号"},
		{"phone too short", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "1380000000"}, "手机号"},
		{"phone too long", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "138000000000"}, "手机号"},
		{"phone with letters", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "1380000000a"}, "手机号"},
		{"phone with fullwidth digits", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "１３８００００００００"}, "手机号"},
		{"phone with trailing newline", service.RegisterInput{Username: "alice", Password: "secret1", Phone: "13800000000\n"}, "手机号"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			u, err := e.svc.Register(context.Background(), tc.in)
			creates, _, _ := e.repo.counts()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Register: %v", err)
				}
				if u.Username != tc.in.Username || u.ID == 0 || u.Role != "user" {
					t.Fatalf("unexpected user %+v", u)
				}
				return
			}
			if !errors.Is(err, apperr.InvalidRequest) {
				t.Fatalf("err = %v, want InvalidRequest", err)
			}
			ae, _ := apperr.As(err)
			if !strings.Contains(ae.Message, tc.wantErr) {
				t.Fatalf("message %q should mention %q", ae.Message, tc.wantErr)
			}
			if creates != 0 {
				t.Fatalf("校验失败不应触达仓储，Create 被调用 %d 次", creates)
			}
			if n := e.hasher.hashCalls.Load(); n != 0 {
				t.Fatalf("校验失败不应做哈希，Hash 被调用 %d 次", n)
			}
		})
	}
}

func TestRegisterStoresHashNotPlaintextAndReturnsNoHash(t *testing.T) {
	e := newEnv(t)
	u, err := e.svc.Register(context.Background(), service.RegisterInput{Username: "alice", Password: "secret1", Phone: "13800000000"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	acc, err := e.repo.FindByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if acc.PasswordHash == "secret1" || acc.PasswordHash != "h:secret1" {
		t.Fatalf("仓储里应存哈希而不是明文: %q", acc.PasswordHash)
	}
	if u.Phone != "13800000000" || u.Status != service.StatusActive {
		t.Fatalf("unexpected user %+v", u)
	}
}

func TestRegisterAlwaysCreatesPlainUserEvenIfCallerWantsMore(t *testing.T) {
	e := newEnv(t)
	u, err := e.svc.Register(context.Background(), service.RegisterInput{Username: "mallory", Password: "secret1", Phone: "13800000000"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.Role != "user" {
		t.Fatalf("注册只能创建普通用户，got role %q", u.Role)
	}
}

func TestRegisterDuplicateUsernameMapsToUserExist(t *testing.T) {
	e := newEnv(t)
	in := service.RegisterInput{Username: "alice", Password: "secret1", Phone: "13800000000"}
	if _, err := e.svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := e.svc.Register(context.Background(), in)
	if !errors.Is(err, apperr.UserExist) {
		t.Fatalf("err = %v, want UserExist", err)
	}
	if ae, _ := apperr.As(err); ae.HTTPStatus != 409 {
		t.Fatalf("status = %d, want 409", ae.HTTPStatus)
	}
}

func TestRegisterConcurrentSameUsernameExactlyOneWins(t *testing.T) {
	e := newEnv(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Register(context.Background(), service.RegisterInput{Username: "same", Password: "secret1", Phone: "13800000000"})
		}()
	}
	wg.Wait()
	ok, exist := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, apperr.UserExist):
			exist++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if ok != 1 || exist != n-1 {
		t.Fatalf("ok=%d exist=%d, want 1 and %d", ok, exist, n-1)
	}
}

func TestRegisterRepoFailureIsSystemErrorNotLeaked(t *testing.T) {
	e := newEnv(t)
	e.repo.failNext = errBoom
	_, err := e.svc.Register(context.Background(), service.RegisterInput{Username: "alice", Password: "secret1", Phone: "13800000000"})
	if err == nil || errors.Is(err, apperr.UserExist) || errors.Is(err, apperr.InvalidRequest) {
		t.Fatalf("err = %v, want infrastructure error", err)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("原因应被包装保留以便日志排查: %v", err)
	}
	if got := apperr.Resolve(err); got.Code != apperr.CodeSysError {
		t.Fatalf("对外应是 SysError, got %d", got.Code)
	}
}
