package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// 所有预定义错误：码值与 HTTP 状态是对外契约（前端、脚本依赖），这里逐个钉死。
var table = []struct {
	err    *Error
	code   int
	status int
}{
	{CoolingDown, 10001, http.StatusTooManyRequests},
	{OutOfStock, 10002, http.StatusConflict},
	{RateLimited, 10003, http.StatusTooManyRequests},
	{InvalidRequest, 10004, http.StatusBadRequest},
	{NotPreheated, 10005, http.StatusConflict},
	{NotStarted, 10006, http.StatusConflict},
	{ActivityEnded, 10007, http.StatusConflict},
	{CouponNotFound, 10008, http.StatusNotFound},
	{CouponLimitExceeded, 10009, http.StatusConflict},
	{RepeatSeckill, 10010, http.StatusConflict},
	{OrderNotFound, 10011, http.StatusNotFound},
	{SeckillUseDedicatedAPI, 10012, http.StatusBadRequest},
	{CouponServiceUnavailable, 10013, http.StatusServiceUnavailable},
	{CouponDisabled, 10014, http.StatusConflict},
	{AuthFail, 20001, http.StatusUnauthorized},
	{TokenInvalid, 20002, http.StatusUnauthorized},
	{UserNotFound, 20003, http.StatusNotFound},
	{UserExist, 20004, http.StatusConflict},
	{PasswordError, 20005, http.StatusUnauthorized},
	{Forbidden, 20006, http.StatusForbidden},
	{SysError, 50000, http.StatusInternalServerError},
	{SystemBusy, 50003, http.StatusServiceUnavailable},
}

func TestPredefinedErrorsKeepCodeAndStatusContract(t *testing.T) {
	seen := map[int]bool{}
	for _, tc := range table {
		if tc.err.Code != tc.code || tc.err.HTTPStatus != tc.status {
			t.Errorf("%s: got code=%d status=%d, want code=%d status=%d",
				tc.err.Message, tc.err.Code, tc.err.HTTPStatus, tc.code, tc.status)
		}
		if seen[tc.code] {
			t.Errorf("duplicate code %d", tc.code)
		}
		seen[tc.code] = true
		if strings.TrimSpace(tc.err.Message) == "" {
			t.Errorf("code %d has empty message", tc.code)
		}
	}
}

// 防止有人新增错误码常量却忘了加进契约表：常量个数与顺序必须和表一致。
func TestEveryCodeConstantIsCoveredByContractTable(t *testing.T) {
	consts := []int{
		CodeCoolingDown, CodeOutOfStock, CodeRateLimited, CodeInvalidRequest, CodeNotPreheated,
		CodeNotStarted, CodeActivityEnded, CodeCouponNotFound, CodeCouponLimitExceeded, CodeRepeatSeckill,
		CodeOrderNotFound, CodeSeckillUseDedicatedAPI, CodeCouponServiceUnavailable, CodeCouponDisabled,
		CodeAuthFail, CodeTokenInvalid, CodeUserNotFound, CodeUserExist, CodePasswordError, CodeForbidden,
		CodeSysError, CodeSystemBusy,
	}
	if len(consts) != len(table) {
		t.Fatalf("contract table has %d entries but %d code constants", len(table), len(consts))
	}
	for i, c := range consts {
		if table[i].code != c {
			t.Errorf("table[%d].code=%d, constant=%d (keep the same order)", i, table[i].code, c)
		}
	}
}

func TestIsMatchesByCodeAcrossCopies(t *testing.T) {
	custom := OutOfStock.WithMessage("该券已抢光").WithCause(errors.New("db: no rows"))
	if !errors.Is(custom, OutOfStock) {
		t.Fatal("a customised copy must still match the predefined error by code")
	}
	if errors.Is(custom, RepeatSeckill) {
		t.Fatal("different codes must not match")
	}
	wrapped := fmt.Errorf("deduct stock: %w", custom)
	if !errors.Is(wrapped, OutOfStock) {
		t.Fatal("errors.Is must see through fmt.Errorf wrapping")
	}
}

func TestWithMessageAndWithCauseDoNotMutatePredefinedErrors(t *testing.T) {
	origMsg := OutOfStock.Message
	_ = OutOfStock.WithMessage("changed").WithCause(errors.New("x"))
	if OutOfStock.Message != origMsg || OutOfStock.Unwrap() != nil {
		t.Fatalf("predefined error was mutated: %+v", OutOfStock)
	}
}

func TestResolveHidesInternalErrors(t *testing.T) {
	raw := errors.New("pq: password authentication failed for user postgres")
	got := Resolve(raw)
	if got.Code != CodeSysError || got.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("non-business error must resolve to SysError, got %+v", got)
	}
	if strings.Contains(got.Message, "password") || strings.Contains(got.Message, "pq:") {
		t.Fatalf("internal detail leaked into the public message: %q", got.Message)
	}
	if !errors.Is(got, raw) {
		t.Fatal("the original error must stay reachable (for logging)")
	}
	if Resolve(nil) != nil {
		t.Fatal("Resolve(nil) must be nil")
	}
}

func TestCodeAndStatusOf(t *testing.T) {
	if CodeOf(nil) != CodeOK || StatusOf(nil) != http.StatusOK {
		t.Fatal("nil error is success")
	}
	if CodeOf(fmt.Errorf("ctx: %w", RateLimited)) != CodeRateLimited {
		t.Fatal("CodeOf must unwrap")
	}
	if StatusOf(errors.New("boom")) != http.StatusInternalServerError {
		t.Fatal("unknown errors are 500")
	}
}

func TestErrorStringIncludesCauseForLogsOnly(t *testing.T) {
	e := SystemBusy.WithCause(errors.New("kafka: ack timeout"))
	if !strings.Contains(e.Error(), "kafka: ack timeout") {
		t.Fatalf("Error() should include the cause for logs: %q", e.Error())
	}
	if strings.Contains(e.Message, "kafka") {
		t.Fatalf("public message must not include the cause: %q", e.Message)
	}
}
