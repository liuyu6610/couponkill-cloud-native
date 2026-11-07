package reservation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// wantErr 断言 err 是 apperr 且业务码与 HTTP 状态都符合。
func wantErr(t *testing.T, err error, code, status int) {
	t.Helper()
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("期望业务错误 code=%d status=%d, 得到 %v", code, status, err)
	}
	if ae.Code != code || ae.HTTPStatus != status {
		t.Fatalf("期望 code=%d status=%d, 得到 code=%d status=%d (%s)", code, status, ae.Code, ae.HTTPStatus, ae.Message)
	}
}

func TestCreateStoresPendingReservationTriggeredAtSaleStart(t *testing.T) {
	e := newEnv(t)
	c := seckillCoupon(1001, e.clock.Now(), time.Hour, 2*time.Hour)
	e.coupons.put(c)

	got, err := e.svc.Create(context.Background(), 7, 1001)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == 0 || got.UserID != 7 || got.CouponID != 1001 || got.Status != StatusPending || got.RetryCount != 0 {
		t.Fatalf("预约内容不符: %+v", got)
	}
	if got.TriggerAt == nil || !got.TriggerAt.Equal(*c.SeckillStartAt) {
		t.Fatalf("trigger_at 应等于开售时间 %v, 得到 %v", c.SeckillStartAt, got.TriggerAt)
	}
	if got.FiredAt != nil || got.RequestID != nil || got.OrderID != nil {
		t.Fatalf("新预约不应有 fired_at/request_id/order_id: %+v", got)
	}
}

func TestCreateValidation(t *testing.T) {
	now := time.Now().UTC()
	hour := time.Hour
	past, future := now.Add(-hour), now.Add(hour)
	far := now.Add(2 * hour)

	cases := []struct {
		name         string
		coupon       *Coupon
		userID       int64
		couponID     int64
		code, status int
	}{
		{"券不存在", nil, 7, 1001, apperr.CodeCouponNotFound, http.StatusNotFound},
		{"普通券", &Coupon{ID: 1001, Type: CouponTypeNormal, Status: CouponStatusEnabled, SeckillStartAt: &future, SeckillEndAt: &far},
			7, 1001, apperr.CodeInvalidRequest, http.StatusBadRequest},
		{"券已停用", &Coupon{ID: 1001, Type: CouponTypeSeckill, Status: CouponStatusDisabled, SeckillStartAt: &future, SeckillEndAt: &far},
			7, 1001, apperr.CodeCouponDisabled, http.StatusConflict},
		{"未配置时间窗", &Coupon{ID: 1001, Type: CouponTypeSeckill, Status: CouponStatusEnabled},
			7, 1001, apperr.CodeInvalidRequest, http.StatusBadRequest},
		{"只配置了开始时间", &Coupon{ID: 1001, Type: CouponTypeSeckill, Status: CouponStatusEnabled, SeckillStartAt: &future},
			7, 1001, apperr.CodeInvalidRequest, http.StatusBadRequest},
		{"已开售", &Coupon{ID: 1001, Type: CouponTypeSeckill, Status: CouponStatusEnabled, SeckillStartAt: &past, SeckillEndAt: &future},
			7, 1001, apperr.CodeInvalidRequest, http.StatusBadRequest},
		{"活动已结束", &Coupon{ID: 1001, Type: CouponTypeSeckill, Status: CouponStatusEnabled, SeckillStartAt: &past, SeckillEndAt: &past},
			7, 1001, apperr.CodeActivityEnded, http.StatusConflict},
		{"用户 ID 非法", nil, 0, 1001, apperr.CodeInvalidRequest, http.StatusBadRequest},
		{"券 ID 非法", nil, 7, 0, apperr.CodeInvalidRequest, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.coupon != nil {
				e.coupons.put(*tc.coupon)
			}
			_, err := e.svc.Create(context.Background(), tc.userID, tc.couponID)
			wantErr(t, err, tc.code, tc.status)
			items, total, err := e.svc.Mine(context.Background(), 7, 1, 10)
			if err != nil || total != 0 || len(items) != 0 {
				t.Fatalf("校验失败不应留下预约: %v total=%d err=%v", items, total, err)
			}
		})
	}
}

func TestCreateWindowBoundaryIsExclusiveAtStart(t *testing.T) {
	e := newEnv(t)
	now := e.clock.Now()
	end := now.Add(time.Hour)
	// 开售时间恰好等于现在：已开售，不可预约
	e.coupons.put(Coupon{ID: 1, Type: CouponTypeSeckill, Status: CouponStatusEnabled, SeckillStartAt: &now, SeckillEndAt: &end})
	_, err := e.svc.Create(context.Background(), 7, 1)
	wantErr(t, err, apperr.CodeInvalidRequest, http.StatusBadRequest)
	// 推进到开售前 1ms：可以预约
	e.clock.Advance(-time.Millisecond)
	if _, err := e.svc.Create(context.Background(), 7, 1); err != nil {
		t.Fatalf("开售前 1ms 应可预约: %v", err)
	}
}

func TestCreateCouponLookupFailureIsServiceUnavailable(t *testing.T) {
	e := newEnv(t)
	e.coupons.err = errBoom
	_, err := e.svc.Create(context.Background(), 7, 1001)
	wantErr(t, err, apperr.CodeCouponServiceUnavailable, http.StatusServiceUnavailable)

	// 业务错误（如下游返回的 503 SystemBusy）原样透传
	e.coupons.err = apperr.SystemBusy
	_, err = e.svc.Create(context.Background(), 7, 1001)
	wantErr(t, err, apperr.CodeSystemBusy, http.StatusServiceUnavailable)
}

func TestCreateRejectsDuplicateActiveReservation(t *testing.T) {
	e := newEnv(t)
	e.coupons.put(seckillCoupon(1001, e.clock.Now(), time.Hour, time.Hour))
	e.coupons.put(seckillCoupon(1002, e.clock.Now(), time.Hour, time.Hour))
	ctx := context.Background()

	first, err := e.svc.Create(ctx, 7, 1001)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Create(ctx, 7, 1001)
	wantErr(t, err, apperr.CodeRepeatSeckill, http.StatusConflict)

	// 其他用户、其他券不受影响
	if _, err := e.svc.Create(ctx, 8, 1001); err != nil {
		t.Fatalf("其他用户应可预约同一张券: %v", err)
	}
	if _, err := e.svc.Create(ctx, 7, 1002); err != nil {
		t.Fatalf("同一用户应可预约其他券: %v", err)
	}
	// 取消后可再次预约（部分唯一索引只覆盖进行中的状态）
	if err := e.svc.Cancel(ctx, 7, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Create(ctx, 7, 1001); err != nil {
		t.Fatalf("取消后应可重新预约: %v", err)
	}
}

func TestCreateConcurrentOnlyOneWins(t *testing.T) {
	e := newEnv(t)
	e.coupons.put(seckillCoupon(1001, e.clock.Now(), time.Hour, time.Hour))
	const n = 12
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = e.svc.Create(context.Background(), 7, 1001)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, apperr.RepeatSeckill):
		default:
			t.Fatalf("并发创建只应得到成功或重复预约, 得到 %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("并发创建应恰好成功 1 个, 实际 %d", ok)
	}
}

func TestCancelOwnershipAndState(t *testing.T) {
	e := newEnv(t)
	e.coupons.put(seckillCoupon(1001, e.clock.Now(), time.Hour, time.Hour))
	ctx := context.Background()
	r, err := e.svc.Create(ctx, 7, 1001)
	if err != nil {
		t.Fatal(err)
	}

	// 别人的预约、不存在的预约：404，且不被改动
	wantErr(t, e.svc.Cancel(ctx, 8, r.ID), apperr.CodeInvalidRequest, http.StatusNotFound)
	wantErr(t, e.svc.Cancel(ctx, 7, 999999), apperr.CodeInvalidRequest, http.StatusNotFound)
	if load(t, e.pool, r.ID).Status != StatusPending {
		t.Fatal("他人取消不得改动预约")
	}

	if err := e.svc.Cancel(ctx, 7, r.ID); err != nil {
		t.Fatalf("属主取消 PENDING 应成功: %v", err)
	}
	if got := load(t, e.pool, r.ID); got.Status != StatusCancelled {
		t.Fatalf("取消后状态 = %s", got.Status)
	}
	// 终态不可再取消
	wantErr(t, e.svc.Cancel(ctx, 7, r.ID), apperr.CodeInvalidRequest, http.StatusConflict)
}

func TestCancelRefusesNonPendingStates(t *testing.T) {
	e := newEnv(t)
	trigger := e.clock.Now()
	id := int64(0)
	for _, st := range []Status{StatusFiring, StatusQueued, StatusSuccess, StatusFailed, StatusExpired} {
		id++
		insertRow(t, e.pool, row{id: id, userID: 7, couponID: id, status: st, triggerAt: &trigger})
		err := e.svc.Cancel(context.Background(), 7, id)
		wantErr(t, err, apperr.CodeInvalidRequest, http.StatusConflict)
		var ae *apperr.Error
		errors.As(err, &ae)
		if want := string(st); !strings.Contains(ae.Message, want) {
			t.Errorf("提示应包含当前状态 %s: %q", want, ae.Message)
		}
		if load(t, e.pool, id).Status != st {
			t.Errorf("%s 不应被取消改动", st)
		}
	}
}

func TestGetAndMineAreScopedToUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, id := range []int64{1001, 1002, 1003} {
		e.coupons.put(seckillCoupon(id, e.clock.Now(), time.Hour, time.Hour))
	}
	var mine []Reservation
	for _, id := range []int64{1001, 1002, 1003} {
		e.clock.Advance(time.Second) // 让 created_at 有先后
		r, err := e.svc.Create(ctx, 7, id)
		if err != nil {
			t.Fatal(err)
		}
		mine = append(mine, r)
	}
	other, err := e.svc.Create(ctx, 8, 1001)
	if err != nil {
		t.Fatal(err)
	}

	got, err := e.svc.Get(ctx, 7, mine[1].ID)
	if err != nil || got.ID != mine[1].ID {
		t.Fatalf("Get 本人预约: %+v err=%v", got, err)
	}
	if _, err := e.svc.Get(ctx, 7, other.ID); err == nil {
		t.Fatal("读取他人的预约应 404")
	} else {
		wantErr(t, err, apperr.CodeInvalidRequest, http.StatusNotFound)
	}

	items, total, err := e.svc.Mine(ctx, 7, 1, 2)
	if err != nil || total != 3 || len(items) != 2 || items[0].ID != mine[2].ID || items[1].ID != mine[1].ID {
		t.Fatalf("Mine 第 1 页应为最新两条: total=%d items=%+v err=%v", total, items, err)
	}
	items, _, _ = e.svc.Mine(ctx, 7, 2, 2)
	if len(items) != 1 || items[0].ID != mine[0].ID {
		t.Fatalf("Mine 第 2 页应只剩最老一条: %+v", items)
	}
}

// ---- OnSeckillResult ----

func queuedRow(t *testing.T, e *env, id, userID, couponID int64, reqID string) {
	t.Helper()
	insertRow(t, e.pool, row{id: id, userID: userID, couponID: couponID, status: StatusQueued, requestID: &reqID})
}

func TestOnSeckillResultSuccessMarksSuccessAndNotifiesOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	queuedRow(t, e, 1, 7, 1001, "rid-1")
	ev := ResultEvent{RequestID: "rid-1", UserID: 7, CouponID: 1001, Success: true, OrderID: 555}

	if err := e.svc.OnSeckillResult(ctx, ev); err != nil {
		t.Fatalf("OnSeckillResult: %v", err)
	}
	got := load(t, e.pool, 1)
	if got.Status != StatusSuccess || got.OrderID == nil || *got.OrderID != 555 {
		t.Fatalf("应为 SUCCESS 且记录订单号: %+v", got)
	}
	notes := notificationsOf(t, e.pool, 7)
	if len(notes) != 1 || notes[0] != notification.TypeReservationSuccess {
		t.Fatalf("应恰好一条成功通知: %v", notes)
	}
	var title, content string
	var ref int64
	if err := e.pool.QueryRow(ctx, `SELECT title, content, ref_id FROM ord.notifications WHERE user_id = 7`).Scan(&title, &content, &ref); err != nil {
		t.Fatal(err)
	}
	if title != "预约帮抢成功" || content != "券 1001 已抢到，订单 555" || ref != 1 {
		t.Fatalf("通知文案/关联不符: %q %q ref=%d", title, content, ref)
	}
}

func TestOnSeckillResultRedeliveryIsIdempotent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	queuedRow(t, e, 1, 7, 1001, "rid-1")
	ev := ResultEvent{RequestID: "rid-1", UserID: 7, CouponID: 1001, Success: true, OrderID: 555}

	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = e.svc.OnSeckillResult(ctx, ev) }()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("重复/并发投递都应成功: %v", err)
		}
	}
	if n := len(notificationsOf(t, e.pool, 7)); n != 1 {
		t.Fatalf("重复投递只能产生一条通知, 得到 %d", n)
	}
	// 终态不被后到的矛盾结果改写
	if err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: "rid-1", UserID: 7, CouponID: 1001, Reason: "OUT_OF_STOCK"}); err != nil {
		t.Fatal(err)
	}
	if got := load(t, e.pool, 1); got.Status != StatusSuccess {
		t.Fatalf("终态不得被改写: %+v", got)
	}
	if n := len(notificationsOf(t, e.pool, 7)); n != 1 {
		t.Fatalf("矛盾结果不得再产生通知, 得到 %d", n)
	}
}

func TestOnSeckillResultFailureRecordsReason(t *testing.T) {
	cases := []struct {
		reason   string
		wantCode int
		wantMsg  string
		content  string
	}{
		{string(events.ReasonOutOfStock), apperr.CodeOutOfStock, "OUT_OF_STOCK", "券 1001：库存不足"},
		{string(events.ReasonRepeat), apperr.CodeRepeatSeckill, "REPEAT", "券 1001：请勿重复秒杀"},
		{"", apperr.CodeSysError, "SYSTEM_ERROR", "券 1001 履约失败，请查看「我的预约」"},
		{"SOMETHING_NEW", apperr.CodeSysError, "SYSTEM_ERROR", "券 1001 履约失败，请查看「我的预约」"},
		{string(events.ReasonSystemError), apperr.CodeSysError, "SYSTEM_ERROR", "券 1001 履约失败，请查看「我的预约」"},
	}
	for _, tc := range cases {
		t.Run("reason="+tc.reason, func(t *testing.T) {
			e := newEnv(t)
			queuedRow(t, e, 1, 7, 1001, "rid-1")
			err := e.svc.OnSeckillResult(context.Background(), ResultEvent{RequestID: "rid-1", UserID: 7, CouponID: 1001, Reason: tc.reason})
			if err != nil {
				t.Fatal(err)
			}
			got := load(t, e.pool, 1)
			if got.Status != StatusFailed || got.FailCode == nil || *got.FailCode != tc.wantCode ||
				got.FailReason == nil || *got.FailReason != tc.wantMsg {
				t.Fatalf("失败原因记录不符: %+v", got)
			}
			var content string
			if err := e.pool.QueryRow(context.Background(),
				`SELECT content FROM ord.notifications WHERE user_id = 7 AND type = $1`, notification.TypeReservationFailed).Scan(&content); err != nil {
				t.Fatalf("应有一条失败通知: %v", err)
			}
			if content != tc.content {
				t.Fatalf("通知内容 = %q, 期望 %q", content, tc.content)
			}
		})
	}
}

func TestOnSeckillResultWithoutReservationIsNoop(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// 手动秒杀：没有任何预约，或预约是别的请求号（QUEUED）
	if err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: "manual", UserID: 7, CouponID: 1001, Success: true, OrderID: 1}); err != nil {
		t.Fatalf("找不到预约应返回 nil: %v", err)
	}
	queuedRow(t, e, 1, 7, 1001, "rid-other")
	if err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: "manual", UserID: 7, CouponID: 1001, Success: true, OrderID: 1}); err != nil {
		t.Fatal(err)
	}
	if got := load(t, e.pool, 1); got.Status != StatusQueued {
		t.Fatalf("请求号不同的结果不得改动 QUEUED 预约: %+v", got)
	}
	if n := len(notificationsOf(t, e.pool, 7)); n != 0 {
		t.Fatalf("不得产生通知, 得到 %d", n)
	}
}

func TestOnSeckillResultBeforeQueuedWritebackMatchesFiringReservation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	fired := e.clock.Now()
	trigger := fired.Add(-time.Second)
	insertRow(t, e.pool, row{id: 1, userID: 7, couponID: 1001, status: StatusFiring, triggerAt: &trigger, firedAt: &fired})

	if err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: "rid-early", UserID: 7, CouponID: 1001, Success: true, OrderID: 9}); err != nil {
		t.Fatal(err)
	}
	got := load(t, e.pool, 1)
	if got.Status != StatusSuccess || got.RequestID == nil || *got.RequestID != "rid-early" {
		t.Fatalf("FIRING 的预约应被兜底匹配并记录 request_id: %+v", got)
	}
	// 同一个事件再投递：此时按 request_id 命中终态行，仍然幂等
	if err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: "rid-early", UserID: 7, CouponID: 1001, Success: true, OrderID: 9}); err != nil {
		t.Fatal(err)
	}
	if n := len(notificationsOf(t, e.pool, 7)); n != 1 {
		t.Fatalf("应只有一条通知, 得到 %d", n)
	}
}

func TestOnSeckillResultRejectsIncompleteEvent(t *testing.T) {
	e := newEnv(t)
	for _, ev := range []ResultEvent{
		{UserID: 7, CouponID: 1},
		{RequestID: "r", CouponID: 1},
		{RequestID: "r", UserID: 7},
	} {
		wantErr(t, e.svc.OnSeckillResult(context.Background(), ev), apperr.CodeInvalidRequest, http.StatusBadRequest)
	}
}
