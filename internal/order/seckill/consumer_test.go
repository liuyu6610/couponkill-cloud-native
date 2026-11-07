package seckill_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/kafkax"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

func (e *env) publish(payload []byte, userID int64) {
	e.t.Helper()
	if err := e.broker.Producer().Publish(context.Background(), events.TopicSeckillOrderCreate, events.SeckillOrderCreateKey(userID), payload); err != nil {
		e.t.Fatalf("publish: %v", err)
	}
}

func (e *env) status(requestID string) string { return e.reqField(requestID, keys.ReqFieldStatus) }

func (e *env) mustOrder(userID int64, requestID string) orders.Order {
	e.t.Helper()
	o, ok, err := orders.FindByRequest(context.Background(), e.pool, userID, requestID)
	if err != nil || !ok {
		e.t.Fatalf("order for request %s: found=%v err=%v", requestID, ok, err)
	}
	return o
}

// 去掉消费者的任一步（读券快照、扣库存、事务建单+通知、置 SUCCESS、通知钩子），这个测试就会红。
func TestConsumerFulfilsAnAcceptedRequestEndToEnd(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 5)
	rid := e.enter(42, couponID)

	e.consumeAll()

	o := e.mustOrder(42, rid)
	if o.CouponID != couponID || o.Status != orders.StatusCreated || o.Source != orders.SourceSeckill ||
		o.CouponName != "秒杀券-1001" || string(o.FaceValue) != "50.00" || string(o.MinSpend) != "100.00" || o.CouponType != 2 {
		t.Fatalf("order = %+v", o)
	}
	if got := o.ExpireTime.Sub(o.GetTime); got != 7*24*time.Hour {
		t.Fatalf("validity = %v, want the coupon's 7 days", got)
	}
	if e.status(rid) != "SUCCESS" || e.reqField(rid, keys.ReqFieldOrderID) != strconv.FormatInt(o.ID, 10) {
		t.Fatalf("request hash status %q orderId %q", e.status(rid), e.reqField(rid, keys.ReqFieldOrderID))
	}
	if len(e.inflight()) != 0 {
		t.Fatalf("inflight = %v, want empty after success", e.inflight())
	}
	if n, _ := e.counter(couponID); n != 4 {
		t.Fatalf("counter = %d, success keeps the reservation (want 4)", n)
	}
	if e.coupon.Effects(rid) != 1 || e.coupon.Stock(couponID) != 4 {
		t.Fatalf("deduct effects %d, DB stock %d", e.coupon.Effects(rid), e.coupon.Stock(couponID))
	}
	var typ, title string
	var ref int64
	if err := e.pool.QueryRow(context.Background(), `SELECT type, title, ref_id FROM ord.notifications WHERE user_id = 42`).Scan(&typ, &title, &ref); err != nil {
		t.Fatalf("notification: %v", err)
	}
	if typ != "SECKILL_SUCCESS" || ref != o.ID {
		t.Fatalf("notification type %q ref %d, want SECKILL_SUCCESS for order %d", typ, ref, o.ID)
	}
	evs := e.hook.forRequest(rid)
	if len(evs) != 1 || !evs[0].Success || evs[0].OrderID != o.ID || evs[0].UserID != 42 || evs[0].CouponID != couponID {
		t.Fatalf("hook events = %+v", evs)
	}

	res, err := e.svc.Result(context.Background(), 42, rid)
	if err != nil || res.Status != events.StatusSuccess || res.OrderID != o.ID {
		t.Fatalf("Result = %+v, %v", res, err)
	}
}

// 去掉 PROCESSING/SUCCESS 的 CAS 或 ON CONFLICT：同一条消息投递多次会多建单或多发通知。
func TestConsumerIsIdempotentUnderDuplicateDelivery(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 10)
	var rids []string
	for u := int64(1); u <= 5; u++ {
		rids = append(rids, e.enter(u, couponID))
	}
	// 每条消息再投递两次（Kafka 至少一次语义下的重复）。
	for i, rid := range rids {
		for range 2 {
			e.publish(command(rid, int64(i+1), couponID, e.clock.Now()), int64(i+1))
		}
	}

	e.consumeAll()

	for i, rid := range rids {
		if got := e.ordersOfRequest(rid); got != 1 {
			t.Fatalf("request %s has %d orders, want exactly 1", rid, got)
		}
		if e.coupon.Effects(rid) != 1 {
			t.Fatalf("request %s deducted %d times", rid, e.coupon.Effects(rid))
		}
		if n := e.notificationCount(int64(i + 1)); n != 1 {
			t.Fatalf("user %d got %d notifications, want 1", i+1, n)
		}
		if got := len(e.hook.forRequest(rid)); got != 1 {
			t.Fatalf("request %s notified the hook %d times", rid, got)
		}
	}
	if e.coupon.Stock(couponID) != 5 {
		t.Fatalf("DB stock = %d, want 10-5", e.coupon.Stock(couponID))
	}
}

// 位点只在处理成功后提交；崩溃在提交之前，重启后整批重投，结果仍然是每个请求恰好一单。
func TestConsumerCrashBeforeCommitThenRedeliveryStillOneOrderPerRequest(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 10)
	var rids []string
	for u := int64(1); u <= 4; u++ {
		rids = append(rids, e.enter(u, couponID))
	}
	e.broker.CrashBeforeCommit(events.GroupSeckillOrderCreate, 1)

	err := e.svc.Consume(context.Background(), e.consumerConfig())
	if !errors.Is(err, testkit.ErrInjectedCrash) {
		t.Fatalf("first run = %v, want the injected crash", err)
	}
	if got := e.broker.Committed(events.GroupSeckillOrderCreate, events.TopicSeckillOrderCreate, 0); got != 0 {
		t.Fatalf("offset advanced to %d despite the crash", got)
	}
	if e.orderCount(couponID, 0) != 4 {
		t.Fatalf("orders after the crashed run = %d, the work itself had completed", e.orderCount(couponID, 0))
	}

	e.consumeAll() // 重启：从未提交位点重投

	for _, rid := range rids {
		if e.ordersOfRequest(rid) != 1 || e.coupon.Effects(rid) != 1 || e.status(rid) != "SUCCESS" {
			t.Fatalf("request %s: orders %d effects %d status %s", rid, e.ordersOfRequest(rid), e.coupon.Effects(rid), e.status(rid))
		}
		if got := len(e.hook.forRequest(rid)); got != 1 {
			t.Fatalf("request %s notified %d times, want once across the crash", rid, got)
		}
	}
}

// 崩溃发生在「订单已提交、SUCCESS 尚未写入」之间：重投时靠 FindByRequest 恢复，不再扣库存、不重复建单。
func TestConsumerResumesAfterCrashBetweenOrderCommitAndSuccess(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	// 模拟前一次处理：已 PROCESSING、已扣库存、订单与通知已提交，随后进程死亡。
	e.consumeAll()
	e.mr.HSet(keys.SeckillRequest(rid), keys.ReqFieldStatus, "PROCESSING")
	e.mr.HDel(keys.SeckillRequest(rid), keys.ReqFieldOrderID)
	if _, err := e.mr.ZAdd(keys.SeckillInflight(), float64(e.clock.Now().UnixMilli()), rid); err != nil {
		t.Fatal(err)
	}
	callsBefore := e.coupon.DeductCalls()

	e.broker.ResetGroup(events.GroupSeckillOrderCreate)
	e.consumeAll()

	if e.status(rid) != "SUCCESS" || e.ordersOfRequest(rid) != 1 || e.notificationCount(42) != 1 {
		t.Fatalf("status %s, orders %d, notifications %d", e.status(rid), e.ordersOfRequest(rid), e.notificationCount(42))
	}
	if e.coupon.DeductCalls() != callsBefore {
		t.Fatalf("recovery called deduct %d more times; an existing order means the stock is already taken", e.coupon.DeductCalls()-callsBefore)
	}
	if len(e.inflight()) != 0 {
		t.Fatalf("inflight = %v", e.inflight())
	}
}

// coupon 提交了扣减却因超时丢了响应：消费者拿到 transient 错误，必须重试而不是补偿；
// 重试时同一 requestId 的 deduct 是幂等的，所以仍然只有一次有效扣减、一个订单。
func TestConsumerRetriesWhenDeductTimesOutAfterCommittingWithoutCompensating(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.coupon.FailDeductAfterCommit(2)

	e.consumeAll()

	if e.coupon.Effects(rid) != 1 || e.coupon.DeductCalls() != 3 {
		t.Fatalf("effects %d over %d calls, want 1 effect over 3 calls (2 lost responses + 1 replay)", e.coupon.Effects(rid), e.coupon.DeductCalls())
	}
	if e.coupon.ReleaseCalls() != 0 {
		t.Fatalf("a transient failure triggered %d stock releases; it must never compensate", e.coupon.ReleaseCalls())
	}
	if e.ordersOfRequest(rid) != 1 || e.status(rid) != "SUCCESS" || e.coupon.Stock(couponID) != 2 {
		t.Fatalf("orders %d status %s stock %d", e.ordersOfRequest(rid), e.status(rid), e.coupon.Stock(couponID))
	}
	if n, _ := e.counter(couponID); n != 2 {
		t.Fatalf("counter = %d, want 2", n)
	}
}

// 重试耗尽进死信，但绝不补偿：请求原样留在在途集合里，库存与 Redis 计数都不动，交给 sweeper。
func TestConsumerExhaustedRetriesGoToDeadLetterWithoutCompensation(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.coupon.FailDeductBeforeCommit(1000)

	cfg := e.consumerConfig()
	cfg.MaxRetries = 2
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.svc.Consume(ctx, cfg) }()
	e.waitCommitted(1)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if dlt := e.broker.Messages(events.TopicSeckillOrderCreateDLT); len(dlt) != 1 {
		t.Fatalf("DLT has %d messages, want 1", len(dlt))
	}
	if e.coupon.ReleaseCalls() != 0 || e.ordersOfRequest(rid) != 0 {
		t.Fatalf("releases %d orders %d: nothing may be compensated or created", e.coupon.ReleaseCalls(), e.ordersOfRequest(rid))
	}
	if n, _ := e.counter(couponID); n != 2 {
		t.Fatalf("counter = %d, the reservation must stay until the sweeper decides (want 2)", n)
	}
	if got := e.inflight(); len(got) != 1 || got[0] != rid || e.status(rid) != "PROCESSING" {
		t.Fatalf("inflight %v status %s", got, e.status(rid))
	}
}

func TestConsumerFailsTerminallyWhenTheDatabaseIsSoldOut(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 0)
	e.mr.Set(keys.Stock(couponID), "5") // Redis 与 DB 漂移：闸门放行了，DB 已售罄
	rid := e.enter(42, couponID)

	e.consumeAll()

	if e.status(rid) != "FAILED" || e.reqField(rid, keys.ReqFieldReason) != string(events.ReasonOutOfStock) {
		t.Fatalf("status %s reason %s", e.status(rid), e.reqField(rid, keys.ReqFieldReason))
	}
	if n, _ := e.counter(couponID); n != 5 {
		t.Fatalf("counter = %d, the failed reservation must be given back (want 5)", n)
	}
	if e.mr.Exists(keys.SeckillUser(couponID, 42)) {
		t.Fatal("the marker must be released so the user can try again")
	}
	if e.ordersOfRequest(rid) != 0 || len(e.inflight()) != 0 {
		t.Fatalf("orders %d inflight %v", e.ordersOfRequest(rid), e.inflight())
	}
	evs := e.hook.forRequest(rid)
	if len(evs) != 1 || evs[0].Success || evs[0].Reason != events.ReasonOutOfStock {
		t.Fatalf("hook events = %+v", evs)
	}
	res, err := e.svc.Result(context.Background(), 42, rid)
	if err != nil || res.Status != events.StatusFailed || res.Reason != events.ReasonOutOfStock {
		t.Fatalf("Result = %+v, %v", res, err)
	}
}

// 唯一冲突路径：用户在别的渠道已持有该券。去掉 IsUniqueViolation 分支，消费者会无限重试；
// 去掉其中的 stock/release，库存永久少一件；保留占位标记，用户才不会被再次放行。
func TestConsumerUniqueViolationFailsAsRepeatReleasesStockAndKeepsTheMarker(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	// 用户 42 通过管理员发券已持有该券（另一个 request_id）。
	now := e.clock.Now()
	if _, inserted, err := orders.Insert(context.Background(), e.pool, orders.NewOrder{
		ID: 9001, UserID: 42, CouponID: couponID, RequestID: "granted-earlier", CouponName: "x", CouponType: 2,
		Source: orders.SourceAdmin, Now: now, ExpireAt: now.Add(time.Hour),
	}); err != nil || !inserted {
		t.Fatalf("seed order: %v %v", inserted, err)
	}
	rid := e.enter(42, couponID)

	e.consumeAll()

	if e.status(rid) != "FAILED" || e.reqField(rid, keys.ReqFieldReason) != string(events.ReasonRepeat) {
		t.Fatalf("status %s reason %s", e.status(rid), e.reqField(rid, keys.ReqFieldReason))
	}
	if e.coupon.Deducted(rid) || e.coupon.Stock(couponID) != 3 {
		t.Fatalf("stock must be released: deducted=%v DB stock %d", e.coupon.Deducted(rid), e.coupon.Stock(couponID))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want the reservation returned (3)", n)
	}
	if !e.mr.Exists(keys.SeckillUser(couponID, 42)) {
		t.Fatal("the marker must be kept: the user really holds an active order")
	}
	if e.orderCount(couponID, 0) != 1 || e.notificationCount(42) != 0 {
		t.Fatalf("orders %d notifications %d: the failed attempt must leave nothing behind", e.orderCount(couponID, 0), e.notificationCount(42))
	}
}

func TestConsumerEnforcesTheHoldingCapsBeforeDeducting(t *testing.T) {
	e := newEnv(t, func(o *seckill.Options) { o.MaxActiveSeckill = 1 })
	e.seed(couponID, 5)
	e.seed(1002, 5)
	first := e.enter(42, couponID)
	e.consumeAll()
	second := e.enter(42, 1002)
	e.consumeAll()

	if e.status(first) != "SUCCESS" {
		t.Fatalf("first request %s", e.status(first))
	}
	if e.status(second) != "FAILED" || e.reqField(second, keys.ReqFieldReason) != string(events.ReasonLimitExceeded) {
		t.Fatalf("second request %s reason %s", e.status(second), e.reqField(second, keys.ReqFieldReason))
	}
	if e.coupon.Effects(second) != 0 {
		t.Fatal("a capped user must not consume DB stock")
	}
	if n, _ := e.counter(1002); n != 5 {
		t.Fatalf("counter(1002) = %d, want it given back", n)
	}
}

// ackLostProducer 把消息真的写进 broker，却报告超时：这正是 kafkax 文档里「Publish 超时记录可能已送达」的情形。
type ackLostProducer struct{ inner kafkax.Producer }

func (p ackLostProducer) Publish(ctx context.Context, topic string, key, value []byte, opts ...kafkax.PublishOption) error {
	if err := p.inner.Publish(ctx, topic, key, value, opts...); err != nil {
		return err
	}
	return context.DeadlineExceeded
}

func (p ackLostProducer) Close() error { return p.inner.Close() }

// 入口把 ack 丢失当失败回滚，但消息其实已送达：消费者读到 FAILED 终态必须直接跳过，
// 不扣库存、不建单，也不能把 Redis 计数再回补一次（rolledBack 保证最多回补一次）。
func TestConsumerSkipsADeliveredMessageWhoseRequestWasAlreadyRolledBack(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	svc := newServiceWithProducer(t, e, ackLostProducer{inner: e.broker.Producer()}, e.hook)

	_, err := svc.Enter(context.Background(), 42, couponID)
	wantCode(t, err, apperr.SystemBusy)
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter after the rollback = %d, want 3", n)
	}
	if len(e.broker.Messages(events.TopicSeckillOrderCreate)) != 1 {
		t.Fatal("the message must have been delivered despite the lost ack")
	}

	e.consumeAll()

	if e.coupon.DeductCalls() != 0 || e.orderCount(couponID, 0) != 0 {
		t.Fatalf("deduct calls %d orders %d: a rolled-back request must not be processed", e.coupon.DeductCalls(), e.orderCount(couponID, 0))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, the rollback must not be applied twice", n)
	}
	if len(e.hook.all()) != 0 {
		t.Fatalf("hook events = %+v, a skipped message has nothing to report", e.hook.all())
	}
}

func TestConsumerSendsMalformedPayloadsToTheDeadLetterTopic(t *testing.T) {
	e := newEnv(t)
	e.publish([]byte(`{"requestId":"bad id","userId":"1","couponId":"1","enqueuedAtMs":1}`), 1)
	e.publish([]byte(`not json`), 2)

	e.consumeAll()

	if got := len(e.broker.Messages(events.TopicSeckillOrderCreateDLT)); got != 2 {
		t.Fatalf("DLT has %d messages, want both malformed ones", got)
	}
	if e.coupon.DeductCalls() != 0 {
		t.Fatal("malformed commands must not reach the coupon service")
	}
}

// 钩子返回错误或 panic 只会被记录，不会让消息处理失败（否则会重试并反复通知）。
func TestConsumerIgnoresFailingAndPanickingHooks(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	e.hook.err = errors.New("hook down")
	e.hook.panics = true
	rid := e.enter(42, couponID)

	e.consumeAll()

	if e.status(rid) != "SUCCESS" || e.ordersOfRequest(rid) != 1 {
		t.Fatalf("status %s orders %d", e.status(rid), e.ordersOfRequest(rid))
	}
	if got := len(e.hook.forRequest(rid)); got != 1 {
		t.Fatalf("hook called %d times, want exactly 1", got)
	}
	if dlt := e.broker.Messages(events.TopicSeckillOrderCreateDLT); len(dlt) != 0 {
		t.Fatalf("DLT = %d messages: a hook failure must not fail the message", len(dlt))
	}
}
