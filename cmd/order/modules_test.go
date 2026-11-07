package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/reservation"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

type fakeReservations struct {
	got []reservation.ResultEvent
	err error
}

func (f *fakeReservations) OnSeckillResult(_ context.Context, ev reservation.ResultEvent) error {
	f.got = append(f.got, ev)
	return f.err
}

func TestResultHookTranslatesSeckillTerminalStates(t *testing.T) {
	svc := &fakeReservations{}
	h := resultHook{svc: svc}

	if err := h.OnSeckillResult(context.Background(), seckill.ResultEvent{
		RequestID: "r1", UserID: 7, CouponID: 1001, Success: true, OrderID: 99,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.OnSeckillResult(context.Background(), seckill.ResultEvent{
		RequestID: "r2", UserID: 8, CouponID: 1001, Reason: events.ReasonOutOfStock,
	}); err != nil {
		t.Fatal(err)
	}
	want := []reservation.ResultEvent{
		{RequestID: "r1", UserID: 7, CouponID: 1001, Success: true, OrderID: 99},
		{RequestID: "r2", UserID: 8, CouponID: 1001, Success: false, Reason: "OUT_OF_STOCK"},
	}
	if len(svc.got) != 2 || svc.got[0] != want[0] || svc.got[1] != want[1] {
		t.Fatalf("translated events = %+v, want %+v", svc.got, want)
	}
}

func TestResultHookPropagatesErrorsSoTheCallerCanLogThem(t *testing.T) {
	boom := errors.New("tx failed")
	h := resultHook{svc: &fakeReservations{err: boom}}
	if err := h.OnSeckillResult(context.Background(), seckill.ResultEvent{RequestID: "r"}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

type fakeEnterer struct {
	userID, couponID int64
	res              seckill.EnterResult
	err              error
}

func (f *fakeEnterer) Enter(_ context.Context, userID, couponID int64) (seckill.EnterResult, error) {
	f.userID, f.couponID = userID, couponID
	return f.res, f.err
}

// 预约服务要先于 order.Server 构建，而秒杀入口只有 order.New 之后才存在：入口绑定之前的调用必须是
// 可重试的瞬态错误（调度器会退避重试），而不是 panic 或把预约判成终态失败。
func TestLateSeckillerIsTransientUntilBound(t *testing.T) {
	late := &lateSeckiller{}
	_, err := late.Enter(context.Background(), 1, 2)
	if !errors.Is(err, apperr.SystemBusy) {
		t.Fatalf("before Bind: err = %v, want SystemBusy (retryable)", err)
	}

	enterer := &fakeEnterer{res: seckill.EnterResult{RequestID: "req-9"}}
	late.Bind(enterer)
	rid, err := late.Enter(context.Background(), 7, 1001)
	if err != nil || rid != "req-9" || enterer.userID != 7 || enterer.couponID != 1001 {
		t.Fatalf("after Bind: rid=%q err=%v user=%d coupon=%d", rid, err, enterer.userID, enterer.couponID)
	}

	enterer.err = apperr.RepeatSeckill
	if _, err := late.Enter(context.Background(), 7, 1001); !errors.Is(err, apperr.RepeatSeckill) {
		t.Fatalf("business errors must pass through untouched, got %v", err)
	}
}

type fakeCoupons struct {
	info internalapi.CouponInfo
	err  error
}

func (f fakeCoupons) GetCoupon(_ context.Context, _ int64) (internalapi.CouponInfo, error) {
	return f.info, f.err
}

func TestCouponLookupMapsCouponFacts(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	l := couponLookup{c: fakeCoupons{info: internalapi.CouponInfo{
		ID: 1001, Type: internalapi.CouponTypeSeckill, Status: internalapi.CouponStatusEnabled,
		SeckillStartAt: &start, SeckillEndAt: &end,
	}}}
	got, err := l.Coupon(context.Background(), 1001)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 1001 || got.Type != reservation.CouponTypeSeckill || got.Status != reservation.CouponStatusEnabled ||
		got.SeckillStartAt == nil || !got.SeckillStartAt.Equal(start) || got.SeckillEndAt == nil || !got.SeckillEndAt.Equal(end) {
		t.Fatalf("mapped coupon = %+v", got)
	}

	// 没有时间窗：两端保持 nil（预约模块据此判断「没有开售时间」）
	l = couponLookup{c: fakeCoupons{info: internalapi.CouponInfo{ID: 1001, Type: internalapi.CouponTypeSeckill, Status: internalapi.CouponStatusEnabled}}}
	got, err = l.Coupon(context.Background(), 1001)
	if err != nil || got.SeckillStartAt != nil || got.SeckillEndAt != nil {
		t.Fatalf("a coupon without a window must keep nil bounds: %+v %v", got, err)
	}
}

func TestCouponLookupKeepsNotFoundRecognisable(t *testing.T) {
	l := couponLookup{c: fakeCoupons{err: apperr.CouponNotFound}}
	if _, err := l.Coupon(context.Background(), 1); !errors.Is(err, apperr.CouponNotFound) {
		t.Fatalf("err = %v, want CouponNotFound (the reservation service maps it to a 404)", err)
	}
	boom := errors.New("connection refused")
	l = couponLookup{c: fakeCoupons{err: boom}}
	if _, err := l.Coupon(context.Background(), 1); !errors.Is(err, boom) {
		t.Fatalf("other errors must pass through: %v", err)
	}
}

// 结果回查读的是 order-core 写的请求哈希：只有终态才算「找到」，处理中的请求让预约继续等。
func TestRedisResultReaderReadsOnlyTerminalStates(t *testing.T) {
	rdb, _ := testkit.Redis(t)
	ctx := context.Background()
	r := redisResultReader{rdb: rdb}

	if _, found, err := r.Result(ctx, "missing"); err != nil || found {
		t.Fatalf("an unknown request must be not-found without error: found=%v err=%v", found, err)
	}

	put := func(rid string, fields map[string]any) {
		t.Helper()
		if err := rdb.HSet(ctx, keys.SeckillRequest(rid), fields).Err(); err != nil {
			t.Fatal(err)
		}
	}
	put("queued", map[string]any{keys.ReqFieldStatus: string(events.StatusQueued), keys.ReqFieldUserID: "7", keys.ReqFieldCouponID: "1001"})
	put("processing", map[string]any{keys.ReqFieldStatus: string(events.StatusProcessing), keys.ReqFieldUserID: "7", keys.ReqFieldCouponID: "1001"})
	for _, rid := range []string{"queued", "processing"} {
		if _, found, err := r.Result(ctx, rid); err != nil || found {
			t.Fatalf("%s is not terminal: found=%v err=%v", rid, found, err)
		}
	}

	put("ok", map[string]any{keys.ReqFieldStatus: string(events.StatusSuccess), keys.ReqFieldUserID: "7", keys.ReqFieldCouponID: "1001", keys.ReqFieldOrderID: "123456789012345678"})
	ev, found, err := r.Result(ctx, "ok")
	if err != nil || !found {
		t.Fatalf("success: found=%v err=%v", found, err)
	}
	if ev != (reservation.ResultEvent{RequestID: "ok", UserID: 7, CouponID: 1001, Success: true, OrderID: 123456789012345678}) {
		t.Fatalf("success event = %+v (an 18-digit order id must survive)", ev)
	}

	put("bad", map[string]any{keys.ReqFieldStatus: string(events.StatusFailed), keys.ReqFieldUserID: "8", keys.ReqFieldCouponID: "1001", keys.ReqFieldReason: string(events.ReasonOutOfStock)})
	ev, found, err = r.Result(ctx, "bad")
	if err != nil || !found {
		t.Fatalf("failed: found=%v err=%v", found, err)
	}
	if ev != (reservation.ResultEvent{RequestID: "bad", UserID: 8, CouponID: 1001, Success: false, Reason: "OUT_OF_STOCK"}) {
		t.Fatalf("failed event = %+v", ev)
	}
}

func TestRedisResultReaderSurfacesRedisErrors(t *testing.T) {
	rdb, mr := testkit.Redis(t)
	mr.Close()
	if _, _, err := (redisResultReader{rdb: rdb}).Result(context.Background(), "r"); err == nil {
		t.Fatal("a Redis outage must be reported, not read as 'not found'")
	}
}
