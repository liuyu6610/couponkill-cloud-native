package seckill_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/seckill"
)

func (e *env) sweeper(opts ...func(*seckill.SweeperOptions)) *seckill.Sweeper {
	so := seckill.SweeperOptions{Stale: 120 * time.Second, Interval: 10 * time.Millisecond}
	for _, o := range opts {
		o(&so)
	}
	return e.svc.NewSweeper(so)
}

func (e *env) sweepOnce(sw *seckill.Sweeper) int {
	e.t.Helper()
	n, err := sw.Once(context.Background())
	if err != nil {
		e.t.Fatalf("sweeper round: %v", err)
	}
	return n
}

// 请求入队后消息迟迟没被消费（或进了死信）：超时后 sweeper 归还 DB 库存、回滚 Redis，置 FAILED(TIMEOUT)。
// 去掉 stock/release，迟到的 deduct 不会被墓碑挡住；去掉 rollback，Redis 计数永久少一件。
func TestSweeperFailsStaleRequestWithoutAnOrderAndReleasesEverything(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	sw := e.sweeper()

	if got := e.sweepOnce(sw); got != 0 {
		t.Fatalf("a fresh request was swept (%d)", got)
	}
	e.clock.Advance(121 * time.Second)
	if got := e.sweepOnce(sw); got != 1 {
		t.Fatalf("swept %d, want 1", got)
	}

	if e.status(rid) != "FAILED" || e.reqField(rid, keys.ReqFieldReason) != string(events.ReasonTimeout) {
		t.Fatalf("status %s reason %s", e.status(rid), e.reqField(rid, keys.ReqFieldReason))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want 3", n)
	}
	if e.mr.Exists(keys.SeckillUser(couponID, 42)) || len(e.inflight()) != 0 {
		t.Fatalf("marker/inflight left behind: marker %v inflight %v", e.mr.Exists(keys.SeckillUser(couponID, 42)), e.inflight())
	}
	if e.coupon.ReleaseCalls() != 1 {
		t.Fatalf("release calls = %d, want 1", e.coupon.ReleaseCalls())
	}
	evs := e.hook.forRequest(rid)
	if len(evs) != 1 || evs[0].Success || evs[0].Reason != events.ReasonTimeout {
		t.Fatalf("hook events = %+v", evs)
	}
	if got := e.sweepOnce(sw); got != 0 {
		t.Fatalf("second round swept %d, finalization must be one-shot", got)
	}

	// 消息终于到了：请求已是终态，消费者跳过；coupon 墓碑也会挡住任何直接的 deduct。
	e.consumeAll()
	if e.coupon.DeductCalls() != 0 || e.orderCount(couponID, 0) != 0 {
		t.Fatalf("late message was processed: deducts %d orders %d", e.coupon.DeductCalls(), e.orderCount(couponID, 0))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d after the late message, want 3", n)
	}
}

// 订单已落库但进程在写 SUCCESS 前死了：sweeper 按落库事实置 SUCCESS，既不归还库存也不回滚。
func TestSweeperMarksSuccessWhenTheOrderAlreadyExists(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.consumeAll()
	order := e.mustOrder(42, rid)
	// 把请求倒回「PROCESSING、仍在途」的崩溃现场。
	e.mr.HSet(keys.SeckillRequest(rid), keys.ReqFieldStatus, "PROCESSING")
	e.mr.HDel(keys.SeckillRequest(rid), keys.ReqFieldOrderID)
	if _, err := e.mr.ZAdd(keys.SeckillInflight(), float64(e.clock.Now().UnixMilli()), rid); err != nil {
		t.Fatal(err)
	}
	hookBefore := len(e.hook.all())
	e.clock.Advance(300 * time.Second)

	if got := e.sweepOnce(e.sweeper()); got != 1 {
		t.Fatalf("swept %d, want 1", got)
	}

	if e.status(rid) != "SUCCESS" || e.reqField(rid, keys.ReqFieldOrderID) != strconv.FormatInt(order.ID, 10) {
		t.Fatalf("status %s orderId %s", e.status(rid), e.reqField(rid, keys.ReqFieldOrderID))
	}
	if e.coupon.ReleaseCalls() != 0 {
		t.Fatalf("release calls = %d: a request with an order owns its stock", e.coupon.ReleaseCalls())
	}
	if n, _ := e.counter(couponID); n != 2 {
		t.Fatalf("counter = %d, want 2 (the order keeps its unit)", n)
	}
	if got := len(e.hook.all()) - hookBefore; got != 1 {
		t.Fatalf("hook notified %d times by the sweeper, want 1", got)
	}
	if len(e.inflight()) != 0 {
		t.Fatalf("inflight = %v", e.inflight())
	}
}

// 消费者领取后 PROCESSING 还很新鲜（不足 stale）：sweeper 不能和一个活跃的消费者赛跑。
func TestSweeperLeavesRequestsThatAreBeingProcessedRecently(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.clock.Advance(100 * time.Second)
	// 消费者在 t+100s 领取（刷新 updatedAt）；再过 60s，从入队算已超 120s，但领取只过了 60s。
	e.mr.HSet(keys.SeckillRequest(rid), keys.ReqFieldStatus, "PROCESSING")
	e.mr.HSet(keys.SeckillRequest(rid), keys.ReqFieldUpdatedAt, strconv.FormatInt(e.clock.Now().UnixMilli(), 10))
	e.clock.Advance(60 * time.Second)

	if got := e.sweepOnce(e.sweeper()); got != 0 {
		t.Fatalf("swept %d, an actively processed request must be left alone", got)
	}
	if e.status(rid) != "PROCESSING" {
		t.Fatalf("status = %s", e.status(rid))
	}
	e.clock.Advance(61 * time.Second) // 现在领取也过了 120s：视为崩溃遗留
	if got := e.sweepOnce(e.sweeper()); got != 1 || e.status(rid) != "FAILED" {
		t.Fatalf("swept %d status %s, want the abandoned request failed", got, e.status(rid))
	}
}

// 归还库存遇到 transient 故障：该请求留在在途集合里，下一轮重试；期间 Redis 状态不被提前改动。
func TestSweeperRetriesNextRoundWhenTheReleaseFails(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.clock.Advance(200 * time.Second)
	e.coupon.FailRelease(1)
	sw := e.sweeper()

	n, err := sw.Once(context.Background())
	if err == nil || n != 0 {
		t.Fatalf("round with a failing release = %d, %v; want an error and nothing finalized", n, err)
	}
	if e.status(rid) != "QUEUED" || len(e.inflight()) != 1 {
		t.Fatalf("status %s inflight %v: a failed release must change nothing in Redis", e.status(rid), e.inflight())
	}
	if got := e.sweepOnce(sw); got != 1 || e.status(rid) != "FAILED" {
		t.Fatalf("second round swept %d status %s", got, e.status(rid))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want 3", n)
	}
}

// 去掉分布式锁，多个实例会同时清扫同一批请求。
func TestSweeperRunsOnlyUnderTheDistributedLock(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.clock.Advance(200 * time.Second)
	if err := e.mr.Set(keys.Lock("seckill-sweeper"), "someone-else"); err != nil {
		t.Fatal(err)
	}
	e.mr.SetTTL(keys.Lock("seckill-sweeper"), time.Minute)

	if got := e.sweepOnce(e.sweeper()); got != 0 || e.status(rid) != "QUEUED" {
		t.Fatalf("swept %d status %s while another instance holds the lock", got, e.status(rid))
	}
	e.mr.Del(keys.Lock("seckill-sweeper"))
	if got := e.sweepOnce(e.sweeper()); got != 1 {
		t.Fatalf("swept %d after the lock was freed, want 1", got)
	}
	if e.mr.Exists(keys.Lock("seckill-sweeper")) {
		t.Fatal("the lock must be released after a round")
	}
}

func TestSweeperHandlesAtMostBatchRequestsPerRoundOldestFirst(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 10)
	var rids []string
	for u := int64(1); u <= 5; u++ {
		rids = append(rids, e.enter(u, couponID))
		e.clock.Advance(time.Second)
	}
	e.clock.Advance(200 * time.Second)
	sw := e.sweeper(func(o *seckill.SweeperOptions) { o.Batch = 2 })

	if got := e.sweepOnce(sw); got != 2 {
		t.Fatalf("round 1 swept %d, want the batch size 2", got)
	}
	if e.status(rids[0]) != "FAILED" || e.status(rids[1]) != "FAILED" || e.status(rids[2]) != "QUEUED" {
		t.Fatalf("oldest-first violated: %s %s %s", e.status(rids[0]), e.status(rids[1]), e.status(rids[2]))
	}
	e.sweepOnce(sw)
	e.sweepOnce(sw)
	for _, rid := range rids {
		if e.status(rid) != "FAILED" {
			t.Fatalf("request %s still %s after three rounds", rid, e.status(rid))
		}
	}
	if n, _ := e.counter(couponID); n != 10 {
		t.Fatalf("counter = %d, want all 10 given back", n)
	}
}

func TestSweeperDropsInflightEntriesWhoseHashIsGoneOrAlreadyTerminal(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	gone := e.enter(1, couponID)
	e.mr.Del(keys.SeckillRequest(gone)) // 哈希过期了
	done := e.enter(2, couponID)
	e.consumeAll()
	if _, err := e.mr.ZAdd(keys.SeckillInflight(), float64(e.clock.Now().UnixMilli()), done); err != nil { // 终态却还在集合里
		t.Fatal(err)
	}
	e.clock.Advance(300 * time.Second)

	if got := e.sweepOnce(e.sweeper()); got != 0 {
		t.Fatalf("swept %d, cleanup of dead entries is not a finalization", got)
	}
	if len(e.inflight()) != 0 {
		t.Fatalf("inflight = %v, want the dead entries dropped", e.inflight())
	}
	if e.status(done) != "SUCCESS" {
		t.Fatalf("a terminal request must not be touched, status %s", e.status(done))
	}
}

// 最危险的赛跑：sweeper 在「库存已扣、订单尚未落库」的窗口里抢先把请求判为 TIMEOUT。
// 迟到的消费者落了订单，置 SUCCESS 时被 CAS 拒绝，必须取消这张孤儿订单并归还库存——
// 结果：订单已取消、请求 FAILED、DB 库存与 Redis 计数各只回补一次。
// 去掉 advance.lua 里对 FAILED 的拒绝，或去掉 lateCleanup，这个测试就会红（订单存活 + 库存已回补 = 超卖）。
func TestLateOrderAfterSweeperTimeoutIsReclaimedSoStockIsNeverOversold(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	msg := e.broker.Messages(events.TopicSeckillOrderCreate)[0]
	sw := e.sweeper()
	fired := false
	e.coupon.OnAfterDeduct(func() {
		if fired {
			return
		}
		fired = true
		e.clock.Advance(200 * time.Second)
		if got := e.sweepOnce(sw); got != 1 {
			t.Errorf("sweeper preempted %d requests, want 1", got)
		}
	})

	if err := e.svc.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if e.status(rid) != "FAILED" || e.reqField(rid, keys.ReqFieldReason) != string(events.ReasonTimeout) {
		t.Fatalf("status %s reason %s: the sweeper's verdict is final", e.status(rid), e.reqField(rid, keys.ReqFieldReason))
	}
	order := e.mustOrder(42, rid)
	if order.Status != orders.StatusCancelled {
		t.Fatalf("late order status = %d, want cancelled", order.Status)
	}
	if e.coupon.Deducted(rid) || e.coupon.Stock(couponID) != 3 {
		t.Fatalf("DB stock %d deducted=%v, want everything returned exactly once", e.coupon.Stock(couponID), e.coupon.Deducted(rid))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want 3 (rolled back exactly once)", n)
	}
	if got := len(e.hook.forRequest(rid)); got != 1 {
		t.Fatalf("hook notified %d times, want only the sweeper's failure", got)
	}
	// 用户随后可以重新抢到这张券。
	if again := e.enter(42, couponID); again == rid {
		t.Fatal("expected a fresh request id")
	}
}

// 与上面相反的交错：消费者已领取（PROCESSING），在调 deduct 之前 sweeper 抢先归还（留墓碑）并判 TIMEOUT。
// 消费者的 deduct 被墓碑拒绝（OutOfStock），但此时请求早已是 FAILED(TIMEOUT)：
// rollback 必须是空操作——原因不被覆盖、计数不会被回补两次、钩子不会重复通知。
// 去掉 rollback.lua 的 rolledBack 幂等保护，这个测试就会红（计数变成 4）。
func TestTombstoneStopsAConsumerThatLostTheRaceBeforeDeductingAndRollbackIsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	msg := e.broker.Messages(events.TopicSeckillOrderCreate)[0]
	sw := e.sweeper()
	fired := false
	e.coupon.OnBeforeDeduct(func() {
		if fired {
			return
		}
		fired = true
		e.clock.Advance(200 * time.Second)
		if got := e.sweepOnce(sw); got != 1 {
			t.Errorf("sweeper preempted %d requests, want 1", got)
		}
	})

	if err := e.svc.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if e.coupon.Effects(rid) != 0 || e.coupon.Stock(couponID) != 3 {
		t.Fatalf("effects %d stock %d: the tombstone must stop the late deduct", e.coupon.Effects(rid), e.coupon.Stock(couponID))
	}
	if e.status(rid) != "FAILED" || e.reqField(rid, keys.ReqFieldReason) != string(events.ReasonTimeout) {
		t.Fatalf("status %s reason %s: the first terminal verdict must stand", e.status(rid), e.reqField(rid, keys.ReqFieldReason))
	}
	if n, _ := e.counter(couponID); n != 3 {
		t.Fatalf("counter = %d, want 3 (rolled back once, not twice)", n)
	}
	if got := len(e.hook.forRequest(rid)); got != 1 {
		t.Fatalf("hook notified %d times, want 1", got)
	}
	if e.orderCount(couponID, 0) != 0 {
		t.Fatal("no order may exist")
	}
}

func TestSweeperRunLoopStopsWhenTheContextIsCancelled(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 3)
	rid := e.enter(42, couponID)
	e.clock.Advance(200 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.sweeper().Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for e.status(rid) != "FAILED" {
		if time.Now().After(deadline) {
			t.Fatal("the run loop never finalized the stale request")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil on cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}
