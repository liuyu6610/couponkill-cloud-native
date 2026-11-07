package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func syncReq(id, available int64) internalapi.SyncRequest {
	return internalapi.SyncRequest{CouponID: id, Available: available, Source: "test", Mode: internalapi.SyncModeSet}
}

func syncReqMode(id, available int64, mode string) internalapi.SyncRequest {
	return internalapi.SyncRequest{CouponID: id, Available: available, Source: "test", Mode: mode}
}

func TestGrantDeductsThenCreatesOrderAndTakesRedisStock(t *testing.T) {
	h := newHarness(t, service.Config{})
	res, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42})
	if err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !strings.HasPrefix(res.RequestID, "grant-") || res.OrderID != 777 {
		t.Fatalf("result: %+v", res)
	}
	if len(h.repo.deducts) != 1 || h.repo.deducts[0].RequestID != res.RequestID || h.repo.deducts[0].UserID != 42 {
		t.Fatalf("stock must be deducted with the grant's requestId: %+v", h.repo.deducts)
	}
	if h.orders.last.RequestID != res.RequestID || h.orders.last.UserID != 42 || h.orders.last.CouponID != 1001 {
		t.Fatalf("order grant request: %+v", h.orders.last)
	}
	if len(h.repo.released) != 0 {
		t.Fatal("a successful grant must not release stock")
	}
	if len(h.cache.taken) != 1 || h.cache.taken[0] != 1001 {
		t.Fatalf("redis counter must follow the DB deduction: %v", h.cache.taken)
	}
}

func TestGrantUsesCallerRequestIDForIdempotentRetry(t *testing.T) {
	h := newHarness(t, service.Config{})
	res, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42, RequestID: "admin-retry-1"})
	if err != nil || res.RequestID != "admin-retry-1" {
		t.Fatalf("Grant: %+v %v", res, err)
	}
	_, err = h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42, RequestID: "bad id with spaces"})
	wantCode(t, err, apperr.InvalidRequest)
}

func TestGrantRejectedOrderReleasesStock(t *testing.T) {
	h := newHarness(t, service.Config{})
	h.orders.err = apperr.RepeatSeckill
	_, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42})
	wantCode(t, err, apperr.RepeatSeckill)
	if len(h.repo.released) != 1 || h.repo.released[0].RequestID != h.repo.deducts[0].RequestID || h.repo.released[0].CouponID != 1001 {
		t.Fatalf("a definite rejection must release the deduction: %+v", h.repo.released)
	}
	if len(h.cache.taken) != 0 {
		t.Fatal("redis counter must be untouched when the grant failed")
	}
}

func TestGrantAmbiguousFailureKeepsDeductionAndAsksForRetry(t *testing.T) {
	for name, cause := range map[string]error{
		"transient":        fmt.Errorf("grant: %w", internalapi.ErrTransient),
		"context deadline": context.DeadlineExceeded,
		"context canceled": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, service.Config{})
			h.orders.err = cause
			_, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42, RequestID: "rid-77"})
			wantCode(t, err, apperr.SystemBusy)
			if len(h.repo.released) != 0 {
				t.Fatal("releasing after an unknown outcome could leave an order without stock")
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) || !strings.Contains(ae.Message, "rid-77") {
				t.Fatalf("the message must carry the requestId for the retry: %v", err)
			}
			if ae.HTTPStatus != http.StatusServiceUnavailable {
				t.Fatalf("status = %d", ae.HTTPStatus)
			}
		})
	}
}

func TestGrantStopsWhenStockCannotBeDeducted(t *testing.T) {
	for name, deductErr := range map[string]*apperr.Error{
		"sold out": apperr.OutOfStock, "missing": apperr.CouponNotFound, "disabled": apperr.CouponDisabled,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, service.Config{})
			h.repo.deductErr = deductErr
			_, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42})
			wantCode(t, err, deductErr)
			if h.orders.calls.Load() != 0 {
				t.Fatal("no order may be created without stock")
			}
			if len(h.repo.released) != 0 {
				t.Fatal("nothing was deducted, so nothing to release")
			}
		})
	}
}

func TestGrantReleaseFailureStillSurfacesTheOriginalBusinessError(t *testing.T) {
	h := newHarness(t, service.Config{})
	h.orders.err = apperr.RepeatSeckill
	h.repo.releaseErr = errBoom
	_, err := h.svc.Grant(context.Background(), service.GrantCommand{CouponID: 1001, UserID: 42})
	wantCode(t, err, apperr.RepeatSeckill)
	if !errors.Is(err, errBoom) {
		t.Fatal("the release failure must stay reachable in the error chain for the logs")
	}
}

func TestGrantRejectedAfterCallerCancelsStillReleases(t *testing.T) {
	h := newHarness(t, service.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	h.orders.err = apperr.CouponDisabled
	cancel() // 客户端在建单被拒的同时断开
	_, err := h.svc.Grant(ctx, service.GrantCommand{CouponID: 1001, UserID: 42})
	wantCode(t, err, apperr.CouponDisabled)
	if len(h.repo.released) != 1 {
		t.Fatal("release must not depend on the caller's context")
	}
}

func TestDeductAndReleaseValidateRequests(t *testing.T) {
	h := newHarness(t, service.Config{})
	_, err := h.svc.Deduct(context.Background(), internalapi.DeductRequest{RequestID: "", CouponID: 1, UserID: 1})
	wantCode(t, err, apperr.InvalidRequest)
	_, err = h.svc.Release(context.Background(), internalapi.ReleaseRequest{RequestID: "r", CouponID: 0})
	wantCode(t, err, apperr.InvalidRequest)
	if len(h.repo.deducts)+len(h.repo.released) != 0 {
		t.Fatal("invalid requests must not reach the repo")
	}
}

func TestDeductMapsRepoResultToContract(t *testing.T) {
	h := newHarness(t, service.Config{})
	resp, err := h.svc.Deduct(context.Background(), internalapi.DeductRequest{RequestID: "r1", CouponID: 5, UserID: 9})
	if err != nil || resp.CouponID != 5 || resp.Bucket != 3 || resp.AlreadyDeducted {
		t.Fatalf("resp: %+v %v", resp, err)
	}
	h.repo.deductErr = apperr.OutOfStock
	_, err = h.svc.Deduct(context.Background(), internalapi.DeductRequest{RequestID: "r2", CouponID: 5, UserID: 9})
	wantCode(t, err, apperr.OutOfStock)
}

func TestInfoMapsCouponToInternalContract(t *testing.T) {
	h := newHarness(t, service.Config{})
	c, _ := h.svc.Create(context.Background(), seckillCmd())
	info, err := h.svc.Info(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ID != c.ID || info.Type != service.TypeSeckill || info.Status != service.StatusEnabled ||
		info.FaceValue.String() != "50.00" || info.MinSpend.String() != "100.50" ||
		info.ValidDays != 7 || info.PerUserLimit != 1 || info.SeckillStartAt == nil || info.SeckillEndAt == nil {
		t.Fatalf("info: %+v", info)
	}
	_, err = h.svc.Info(context.Background(), 99999)
	wantCode(t, err, apperr.CouponNotFound)
}

func TestNewRequiresMandatoryDependencies(t *testing.T) {
	if _, err := service.New(service.Deps{}); err == nil {
		t.Fatal("New must reject missing dependencies")
	}
}

func TestNormalizeMoney(t *testing.T) {
	ok := map[string]string{"0": "0.00", "50": "50.00", "7.5": "7.50", "12.34": "12.34", " 3 ": "3.00", "99999999.99": "99999999.99"}
	for in, want := range ok {
		got, err := service.NormalizeMoney(in)
		if err != nil || got != want {
			t.Errorf("NormalizeMoney(%q) = %q,%v want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-1", "1.", ".5", "1.234", "abc", "1e3", "100000000", "+5"} {
		if _, err := service.NormalizeMoney(in); err == nil {
			t.Errorf("NormalizeMoney(%q) must fail", in)
		}
	}
}
