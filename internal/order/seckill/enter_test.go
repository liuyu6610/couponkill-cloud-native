package seckill_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders/orderstest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func wantCode(t *testing.T, err error, want *apperr.Error) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want %d %s", want.Code, want.Message)
	}
	if got := apperr.CodeOf(err); got != want.Code {
		t.Fatalf("error code = %d (%v), want %d (%s)", got, err, want.Code, want.Message)
	}
}

// 去掉 enter.lua 的任一步（DECR / 占位 / 请求哈希 / ZADD）或发布步骤，这个测试就会红。
func TestEnterReservesStockRecordsRequestAndPublishesCommand(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 5)

	rid := e.enter(42, couponID)

	if n, _ := e.counter(couponID); n != 4 {
		t.Fatalf("counter = %d, want 4", n)
	}
	if got, _ := e.mr.Get(keys.SeckillUser(couponID, 42)); got != rid {
		t.Fatalf("user marker = %q, want the requestId %q", got, rid)
	}
	if ttl := e.mr.TTL(keys.SeckillUser(couponID, 42)); ttl != keys.SeckillMarkerTTL {
		t.Fatalf("marker TTL = %v, want %v", ttl, keys.SeckillMarkerTTL)
	}
	if e.reqField(rid, keys.ReqFieldStatus) != "QUEUED" || e.reqField(rid, keys.ReqFieldUserID) != "42" {
		t.Fatalf("request hash = %v", e.mr.HGet(keys.SeckillRequest(rid), keys.ReqFieldStatus))
	}
	if ttl := e.mr.TTL(keys.SeckillRequest(rid)); ttl != keys.SeckillRequestTTL {
		t.Fatalf("request TTL = %v, want %v", ttl, keys.SeckillRequestTTL)
	}
	if got := e.inflight(); len(got) != 1 || got[0] != rid {
		t.Fatalf("inflight = %v", got)
	}
	score, _ := e.mr.ZScore(keys.SeckillInflight(), rid)
	if int64(score) != e.clock.Now().UnixMilli() {
		t.Fatalf("inflight score = %v, want enqueue time %d", score, e.clock.Now().UnixMilli())
	}

	msgs := e.broker.Messages(events.TopicSeckillOrderCreate)
	if len(msgs) != 1 || string(msgs[0].Key) != "42" {
		t.Fatalf("published messages = %+v, want one keyed by userId", msgs)
	}
	cmd, err := events.UnmarshalSeckillOrderCreate(msgs[0].Value)
	if err != nil || cmd.RequestID != rid || cmd.UserID != 42 || cmd.CouponID != couponID || cmd.EnqueuedAtMs != e.clock.Now().UnixMilli() {
		t.Fatalf("command = %+v err %v", cmd, err)
	}
}

func TestEnterMapsLuaReturnCodesToBusinessErrors(t *testing.T) {
	t.Run("not preheated: the stock key does not exist", func(t *testing.T) {
		e := newEnv(t)
		e.coupon.AddCoupon(orderstest.SeckillCoupon(couponID), 5) // 没有写 ck:stock
		_, err := e.svc.Enter(context.Background(), 42, couponID)
		wantCode(t, err, apperr.NotPreheated)
		if len(e.broker.Messages(events.TopicSeckillOrderCreate)) != 0 || len(e.inflight()) != 0 {
			t.Fatal("a rejected enter must leave no trace")
		}
	})
	t.Run("sold out: counter at zero", func(t *testing.T) {
		e := newEnv(t)
		e.seed(couponID, 1)
		e.enter(1, couponID)
		_, err := e.svc.Enter(context.Background(), 2, couponID)
		wantCode(t, err, apperr.OutOfStock)
		if n, _ := e.counter(couponID); n != 0 {
			t.Fatalf("counter = %d, must stay at 0", n)
		}
	})
	t.Run("repeat: the user already holds a marker", func(t *testing.T) {
		e := newEnv(t)
		e.seed(couponID, 5)
		e.enter(42, couponID)
		_, err := e.svc.Enter(context.Background(), 42, couponID)
		wantCode(t, err, apperr.RepeatSeckill)
		if n, _ := e.counter(couponID); n != 4 {
			t.Fatalf("counter = %d, a repeat must not consume a second unit", n)
		}
	})
	t.Run("cooling down", func(t *testing.T) {
		e := newEnv(t, func(o *seckill.Options) { o.Cooldown = 2 * time.Second })
		e.seed(couponID, 5)
		e.seed(1002, 5)
		e.enter(42, couponID)
		e.mr.Del(keys.SeckillUser(couponID, 42)) // 只留冷却键：模拟占位已释放但仍在冷却
		_, err := e.svc.Enter(context.Background(), 42, couponID)
		wantCode(t, err, apperr.CoolingDown)
		if _, err := e.svc.Enter(context.Background(), 43, couponID); err != nil {
			t.Fatalf("cooldown is per user, another user got %v", err)
		}
	})
}

func TestEnterChecksTheCouponBeforeTouchingRedis(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		mutate func(*orderstest.FakeCoupon)
		want   *apperr.Error
	}{
		{"disabled", func(f *orderstest.FakeCoupon) { f.SetStatus(couponID, 0) }, apperr.CouponDisabled},
		{"not started", func(f *orderstest.FakeCoupon) {
			c := orderstest.SeckillCoupon(couponID)
			c.SeckillStartAt, c.SeckillEndAt = orderstest.Window(now.Add(time.Hour)), orderstest.Window(now.Add(2*time.Hour))
			f.AddCoupon(c, 5)
		}, apperr.NotStarted},
		{"ended", func(f *orderstest.FakeCoupon) {
			c := orderstest.SeckillCoupon(couponID)
			c.SeckillStartAt, c.SeckillEndAt = orderstest.Window(now.Add(-2*time.Hour)), orderstest.Window(now.Add(-time.Hour))
			f.AddCoupon(c, 5)
		}, apperr.ActivityEnded},
		{"not a seckill coupon", func(f *orderstest.FakeCoupon) { f.AddCoupon(orderstest.NormalCoupon(couponID), 5) }, apperr.InvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, func(o *seckill.Options) { o.Now = time.Now })
			e.seed(couponID, 5)
			tc.mutate(e.coupon)
			e.mr.Del(keys.CouponMeta(couponID))
			_, err := e.svc.Enter(context.Background(), 42, couponID)
			wantCode(t, err, tc.want)
			if n, _ := e.counter(couponID); n != 5 || len(e.inflight()) != 0 {
				t.Fatalf("a rejected coupon check must not reserve stock (counter %d, inflight %v)", n, e.inflight())
			}
		})
	}

	t.Run("unknown coupon surfaces coupon not found", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.svc.Enter(context.Background(), 42, 999)
		wantCode(t, err, apperr.CouponNotFound)
	})
}

// 去掉缓存回填，第二次秒杀仍会回源 coupon 服务；去掉 id 校验，错放的缓存会被当成这张券。
func TestEnterReadsCouponMetaThroughACacheThatExpiresAfterFiveMinutes(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 5)

	e.enter(1, couponID)
	e.enter(2, couponID)
	if got := e.coupon.GetCalls(); got != 1 {
		t.Fatalf("coupon service was read %d times, want 1 (second call served by cache)", got)
	}
	if ttl := e.mr.TTL(keys.CouponMeta(couponID)); ttl != keys.CouponMetaTTL {
		t.Fatalf("meta TTL = %v, want %v", ttl, keys.CouponMetaTTL)
	}

	// 缓存里放了别的券的内容：不能信，要回源并覆盖。
	if err := e.mr.Set(keys.CouponMeta(couponID), `{"id":"777","name":"x","type":2,"status":1}`); err != nil {
		t.Fatal(err)
	}
	e.enter(3, couponID)
	if got := e.coupon.GetCalls(); got != 2 {
		t.Fatalf("a cache entry for another coupon must be refetched, coupon reads = %d", got)
	}

	// coupon 服务挂了但缓存还在：热路径不受影响；缓存没了则返回 503。
	e.coupon.FailGet(100)
	e.enter(4, couponID)
	e.mr.Del(keys.CouponMeta(couponID))
	_, err := e.svc.Enter(context.Background(), 5, couponID)
	wantCode(t, err, apperr.SystemBusy)
}

// 去掉 Publish 失败后的 rollback.lua，库存永久少一件、用户被占位卡住；回滚只做一次。
func TestEnterRollsBackWhenPublishFails(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	e.broker.FailPublishes(1)

	_, err := e.svc.Enter(context.Background(), 42, couponID)
	wantCode(t, err, apperr.SystemBusy)

	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want it restored to 3", n)
	}
	if e.mr.Exists(keys.SeckillUser(couponID, 42)) {
		t.Fatal("the user marker must be released so the user can retry")
	}
	if len(e.inflight()) != 0 {
		t.Fatalf("inflight = %v, want empty", e.inflight())
	}
	// 失败请求的哈希保留为 FAILED/ENQUEUE_FAILED（可诊断），且之后用户可以再来一次。
	var failedRID string
	for _, k := range e.mr.Keys() {
		if len(k) > len("ck:sk:req:") && k[:len("ck:sk:req:")] == "ck:sk:req:" {
			failedRID = k[len("ck:sk:req:"):]
		}
	}
	if failedRID == "" || e.reqField(failedRID, keys.ReqFieldStatus) != "FAILED" || e.reqField(failedRID, keys.ReqFieldReason) != string(events.ReasonEnqueueFailed) {
		t.Fatalf("failed request %q hash = status %q reason %q", failedRID, e.reqField(failedRID, keys.ReqFieldStatus), e.reqField(failedRID, keys.ReqFieldReason))
	}
	e.enter(42, couponID)
	if n, _ := e.counter(couponID); n != 2 {
		t.Fatalf("counter after the retry = %d, want 2", n)
	}
}

// 去掉发布上的有界等待，一次永不返回的 ack 会把整个入口拖死。
func TestEnterBoundsTheWaitForTheBrokerAck(t *testing.T) {
	e := newEnv(t, func(o *seckill.Options) { o.EnqueueTimeout = 40 * time.Millisecond })
	e.seed(couponID, 3)
	slow := &blockingProducer{}
	svc := newServiceWithProducer(t, e, slow)

	start := time.Now()
	_, err := svc.Enter(context.Background(), 42, couponID)
	wantCode(t, err, apperr.SystemBusy)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Enter blocked for %v on a hung broker", elapsed)
	}
	if !errors.Is(slow.err, context.DeadlineExceeded) {
		t.Fatalf("producer saw %v, want the enqueue deadline", slow.err)
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want the reservation rolled back to 3", n)
	}
}

func TestEnterRejectsNonPositiveIDs(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Enter(context.Background(), 0, couponID)
	wantCode(t, err, apperr.InvalidRequest)
	_, err = e.svc.Enter(context.Background(), 1, -1)
	wantCode(t, err, apperr.InvalidRequest)
}

// 种子券 1001 没有时间窗：开始/结束时间为 null 必须视为「始终开放」，只设一端则只约束那一端。
// 把 nil 当成「未开始」或「已结束」，或者要求两端同时存在才校验，这个测试就会红。
func TestEnterTreatsMissingWindowBoundsAsOpen(t *testing.T) {
	now := time.Now()
	past, future := orderstest.Window(now.Add(-time.Hour)), orderstest.Window(now.Add(time.Hour))
	cases := []struct {
		name       string
		start, end *time.Time
		want       *apperr.Error // nil = 受理
	}{
		{"no window at all", nil, nil, nil},
		{"only a start in the past", past, nil, nil},
		{"only a start in the future", future, nil, apperr.NotStarted},
		{"only an end in the future", nil, future, nil},
		{"only an end in the past", nil, past, apperr.ActivityEnded},
		{"inside a full window", past, future, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, func(o *seckill.Options) { o.Now = time.Now })
			c := orderstest.SeckillCoupon(couponID)
			c.SeckillStartAt, c.SeckillEndAt = tc.start, tc.end
			e.coupon.AddCoupon(c, 5)
			e.mr.Set(keys.Stock(couponID), "5")
			_, err := e.svc.Enter(context.Background(), 42, couponID)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Enter = %v, want accepted", err)
				}
				return
			}
			wantCode(t, err, tc.want)
		})
	}
}
