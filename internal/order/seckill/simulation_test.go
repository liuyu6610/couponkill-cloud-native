package seckill_test

// 秒杀模拟：把 §9 的不变式放到并发、重放、崩溃、响应丢失一起发生的场景里验证。
//
// 不变式：
//   I1 每个 requestId 至多一次有效库存扣减、至多一个订单；
//   I2 守恒：Redis 预扣计数 + 在途请求数 + 成功订单数 = 初始库存，计数永不为负；
//   I3 重复投递、消费者崩溃、coupon 响应丢失都不破坏 I1/I2；
//   I4 transient 失败不触发补偿（库存归还调用数为 0）。

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// liveOrders 返回未取消订单数（= 成功订单数）。
func (e *env) liveOrders(couponID int64) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM ord.orders WHERE coupon_id = $1 AND status <> 4`, couponID).Scan(&n); err != nil {
		e.t.Fatalf("count live orders: %v", err)
	}
	return n
}

// assertConserved 在静止点检查 I2：计数 + 在途 + 成功订单 = 初始库存。
func (e *env) assertConserved(couponID int64, initial int) {
	e.t.Helper()
	counter, _ := e.counter(couponID)
	inflight, success := len(e.inflight()), e.liveOrders(couponID)
	if counter < 0 || counter+inflight+success != initial {
		e.t.Fatalf("conservation broken: counter %d + inflight %d + success %d = %d, want %d", counter, inflight, success, counter+inflight+success, initial)
	}
}

// partitionTargets 返回每个分区上当前消息总数（位点追平的目标）。
func (e *env) partitionTargets() map[int32]int64 {
	out := map[int32]int64{}
	for _, m := range e.broker.Messages(events.TopicSeckillOrderCreate) {
		if m.Offset+1 > out[m.Partition] {
			out[m.Partition] = m.Offset + 1
		}
	}
	return out
}

func (e *env) allCommitted(targets map[int32]int64) bool {
	for p, n := range targets {
		if e.broker.Committed(events.GroupSeckillOrderCreate, events.TopicSeckillOrderCreate, p) < n {
			return false
		}
	}
	return true
}

// drain 反复启动消费者（遇到注入的崩溃就「重启」）直到所有分区的位点追平。
func (e *env) drain(concurrency, maxRetries int) {
	e.t.Helper()
	targets := e.partitionTargets()
	deadline := time.Now().Add(90 * time.Second)
	for !e.allCommitted(targets) {
		if time.Now().After(deadline) {
			e.t.Fatalf("consumers did not drain the topic in time (targets %v)", targets)
		}
		cfg := e.consumerConfig()
		cfg.Concurrency, cfg.MaxRetries = concurrency, maxRetries
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- e.svc.Consume(ctx, cfg) }()
	wait:
		for {
			select {
			case err := <-done:
				cancel()
				if err != nil && !errors.Is(err, testkit.ErrInjectedCrash) {
					e.t.Fatalf("consumer: %v", err)
				}
				break wait
			case <-time.After(2 * time.Millisecond):
				if e.allCommitted(targets) {
					cancel()
					if err := <-done; err != nil && !errors.Is(err, testkit.ErrInjectedCrash) {
						e.t.Fatalf("consumer: %v", err)
					}
					break wait
				}
			}
		}
	}
}

// 500 个用户并发抢 100 件：恰好 100 个受理，其余售罄；Redis 计数全程不为负，静止点守恒。
// 去掉 enter.lua 里的库存判断/DECR 的原子性（例如拆成两次 Redis 调用），超卖或计数为负，这个测试就会红。
func TestSimulation500ConcurrentEntersOnStock100(t *testing.T) {
	const stock, users = 100, 500
	e := newEnv(t)
	e.seed(couponID, stock)

	var minCounter atomic.Int64
	minCounter.Store(stock)
	stop := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if v, err := e.mr.Get(keys.Stock(couponID)); err == nil {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil && n < minCounter.Load() {
					minCounter.Store(n)
				}
			}
		}
	}()

	var accepted, soldOut atomic.Int32
	rids := make(chan string, users)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for u := int64(1); u <= users; u++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := e.svc.Enter(context.Background(), u, couponID)
			switch {
			case err == nil:
				accepted.Add(1)
				rids <- res.RequestID
			case errors.Is(err, apperr.OutOfStock):
				soldOut.Add(1)
			default:
				t.Errorf("user %d: unexpected error %v", u, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(stop)
	watcher.Wait()
	close(rids)

	if accepted.Load() != stock || soldOut.Load() != users-stock {
		t.Fatalf("accepted %d soldOut %d, want exactly %d and %d", accepted.Load(), soldOut.Load(), stock, users-stock)
	}
	if minCounter.Load() < 0 {
		t.Fatalf("the Redis counter dipped to %d", minCounter.Load())
	}
	if n, _ := e.counter(couponID); n != 0 {
		t.Fatalf("counter = %d, want 0", n)
	}
	if got := len(e.broker.Messages(events.TopicSeckillOrderCreate)); got != stock {
		t.Fatalf("%d commands published, want %d", got, stock)
	}
	if got := e.coupon.GetCalls(); got > 25 {
		t.Fatalf("the coupon service was read %d times for 500 concurrent requests; the meta cache should absorb them", got)
	}
	e.assertConserved(couponID, stock) // 0 + 100 在途 + 0 成功

	e.consumeAll()
	e.assertConserved(couponID, stock) // 0 + 0 + 100
	if e.liveOrders(couponID) != stock || e.coupon.TotalEffects() != stock || e.coupon.Stock(couponID) != 0 {
		t.Fatalf("orders %d effects %d DB stock %d", e.liveOrders(couponID), e.coupon.TotalEffects(), e.coupon.Stock(couponID))
	}
}

// 一场带全部故障的秒杀：入口发布失败、每条消息重复投递两次、coupon 响应丢失/请求失败、消费者崩溃两次。
// 结局必须仍然是：恰好 100 个订单、每个 requestId 一个订单一次有效扣减、守恒、没有任何补偿调用。
// 去掉 ON CONFLICT、advance 的 CAS、deduct 的幂等账本或「transient 不补偿」中的任何一个，这个测试都会红。
func TestSimulationFlashSaleUnderEveryFailureStillSellsExactlyTheStock(t *testing.T) {
	const stock, users = 100, 300
	e := newEnvWithBroker(t, testkit.NewFakeBroker(testkit.WithPartitions(4)))
	e.seed(couponID, stock)
	e.broker.FailPublishes(15)

	accepted := map[int64]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for u := int64(1); u <= users; u++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := e.svc.Enter(context.Background(), u, couponID)
			if err == nil {
				mu.Lock()
				accepted[u] = res.RequestID
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	e.assertConserved(couponID, stock)

	// 被拒绝（售罄时抢占了被回滚的名额、或入队失败）的用户补一轮重试，直到卖完。
	for u := int64(1); u <= users; u++ {
		if _, ok := accepted[u]; ok {
			continue
		}
		if res, err := e.svc.Enter(context.Background(), u, couponID); err == nil {
			accepted[u] = res.RequestID
		}
	}
	if len(accepted) != stock {
		t.Fatalf("accepted %d users, want exactly the stock %d", len(accepted), stock)
	}
	if n, _ := e.counter(couponID); n != 0 {
		t.Fatalf("counter = %d, want 0", n)
	}
	e.assertConserved(couponID, stock)

	// 每条消息再投递两次；coupon 丢响应/失败；消费者崩溃两次。
	for u, rid := range accepted {
		for range 2 {
			e.publish(command(rid, u, couponID, e.clock.Now()), u)
		}
	}
	e.coupon.FailDeductAfterCommit(25)
	e.coupon.FailDeductBeforeCommit(10)
	e.broker.CrashBeforeCommit(events.GroupSeckillOrderCreate, 2)

	e.drain(4, 100)

	if e.liveOrders(couponID) != stock || e.orderCount(couponID, 0) != stock {
		t.Fatalf("orders: live %d total %d, want exactly %d", e.liveOrders(couponID), e.orderCount(couponID, 0), stock)
	}
	for u, rid := range accepted {
		if e.ordersOfRequest(rid) != 1 {
			t.Fatalf("user %d request %s has %d orders", u, rid, e.ordersOfRequest(rid))
		}
		if e.coupon.Effects(rid) != 1 {
			t.Fatalf("user %d request %s: %d effective deducts", u, rid, e.coupon.Effects(rid))
		}
		if e.status(rid) != "SUCCESS" {
			t.Fatalf("request %s status %s", rid, e.status(rid))
		}
		if got := len(e.hook.forRequest(rid)); got != 1 {
			t.Fatalf("request %s notified the hook %d times", rid, got)
		}
	}
	if e.coupon.TotalEffects() != stock || e.coupon.Held() != stock || e.coupon.Stock(couponID) != 0 {
		t.Fatalf("effects %d held %d DB stock %d", e.coupon.TotalEffects(), e.coupon.Held(), e.coupon.Stock(couponID))
	}
	if calls := e.coupon.DeductCalls(); calls < stock+35 {
		t.Fatalf("only %d deduct calls: the injected failures (35) and replays never happened, the test proves nothing", calls)
	}
	if e.coupon.ReleaseCalls() != 0 {
		t.Fatalf("%d stock releases: transient failures must never be compensated", e.coupon.ReleaseCalls())
	}
	if dlt := e.broker.Messages(events.TopicSeckillOrderCreateDLT); len(dlt) != 0 {
		t.Fatalf("%d messages dead-lettered", len(dlt))
	}
	if n, _ := e.counter(couponID); n != 0 || len(e.inflight()) != 0 {
		t.Fatalf("counter %d inflight %v", n, e.inflight())
	}
	e.assertConserved(couponID, stock)

	n, err := e.sweeper().Once(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("sweeper after a clean drain = %d, %v; there is nothing to finalize", n, err)
	}
}

// 同一条消息被并发处理（再平衡时两个消费者同时拿到）：只会有一个订单、一次扣减、一次通知，且请求不会被误判失败。
// PostgreSQL 在并发重放时可能先撞上部分唯一索引而不是 ON CONFLICT 的仲裁索引，
// 去掉「按请求号回查订单」的区分，重放会被误当成 REPEAT 失败。
func TestSimulationConcurrentDuplicateProcessingOfOneMessage(t *testing.T) {
	for round := range 3 {
		e := newEnv(t)
		e.seed(couponID, 3)
		rid := e.enter(42, couponID)
		msg := e.broker.Messages(events.TopicSeckillOrderCreate)[0]

		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := e.svc.Handle(context.Background(), msg); err != nil {
					t.Errorf("Handle: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()

		if e.status(rid) != "SUCCESS" || e.ordersOfRequest(rid) != 1 || e.coupon.Effects(rid) != 1 || e.notificationCount(42) != 1 {
			t.Fatalf("round %d: status %s orders %d effects %d notifications %d", round, e.status(rid), e.ordersOfRequest(rid), e.coupon.Effects(rid), e.notificationCount(42))
		}
		if got := len(e.hook.forRequest(rid)); got != 1 {
			t.Fatalf("round %d: hook notified %d times", round, got)
		}
		if e.coupon.ReleaseCalls() != 0 {
			t.Fatalf("round %d: %d stock releases for a request that succeeded", round, e.coupon.ReleaseCalls())
		}
		e.assertConserved(couponID, 3)
	}
}

func (e *env) orderService() *orders.Service {
	e.t.Helper()
	ids, err := idgen.New(5)
	if err != nil {
		e.t.Fatal(err)
	}
	svc, err := orders.New(orders.Deps{DB: e.pool, Coupons: e.svc.Meta(), Stock: e.coupon.Client(e.t), Restorer: e.svc, IDs: ids},
		orders.Options{Now: e.clock.Now})
	if err != nil {
		e.t.Fatal(err)
	}
	return svc
}

// 取消后库存与占位归还，用户可以再抢，守恒始终成立。
// 去掉取消流程里的 Restore（release 或 rollback.lua），再抢会被占位标记/计数挡住，这个测试就会红。
func TestSimulationCancelThenSeckillAgainSucceeds(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 1)
	svc := e.orderService()

	first := e.enter(42, couponID)
	e.consumeAll()
	o := e.mustOrder(42, first)
	if _, err := e.svc.Enter(context.Background(), 43, couponID); !errors.Is(err, apperr.OutOfStock) {
		t.Fatalf("the only unit is taken, got %v", err)
	}

	cancelled, err := svc.Cancel(context.Background(), 42, o.ID)
	if err != nil || !cancelled {
		t.Fatalf("Cancel = %v, %v", cancelled, err)
	}
	if n, _ := e.counter(couponID); n != 1 || e.coupon.Stock(couponID) != 1 || e.mr.Exists(keys.SeckillUser(couponID, 42)) {
		t.Fatalf("counter %d DB stock %d marker %v: the cancel must return everything", n, e.coupon.Stock(couponID), e.mr.Exists(keys.SeckillUser(couponID, 42)))
	}
	e.assertConserved(couponID, 1)

	second := e.enter(42, couponID)
	e.consumeAll()
	if e.status(second) != "SUCCESS" || second == first {
		t.Fatalf("second request %s status %s", second, e.status(second))
	}
	again := e.mustOrder(42, second)
	if again.ID == o.ID || again.Status != orders.StatusCreated {
		t.Fatalf("second order = %+v", again)
	}
	if n, _ := e.counter(couponID); n != 0 || e.coupon.Stock(couponID) != 0 {
		t.Fatalf("counter %d DB stock %d, want 0 and 0", n, e.coupon.Stock(couponID))
	}
	e.assertConserved(couponID, 1)
}

// 对同一张秒杀订单并发取消十二次：库存只回补一次（计数 = 初始值，不是更多），账本只有一次归还。
// 去掉 rollback.lua 的 rolledBack 幂等标记，计数会被回补多次，这个测试就会红。
func TestSimulationRacingCancelsRestoreStockExactlyOnce(t *testing.T) {
	const stock = 5
	e := newEnv(t)
	e.seed(couponID, stock)
	svc := e.orderService()
	rid := e.enter(42, couponID)
	e.consumeAll()
	o := e.mustOrder(42, rid)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if ok, err := svc.Cancel(context.Background(), 42, o.ID); err != nil || !ok {
				t.Errorf("Cancel = %v, %v", ok, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n, _ := e.counter(couponID); n != stock {
		t.Fatalf("counter = %d, want %d: the restore must be applied exactly once", n, stock)
	}
	if e.coupon.Stock(couponID) != stock || e.coupon.Held() != 0 {
		t.Fatalf("DB stock %d held %d", e.coupon.Stock(couponID), e.coupon.Held())
	}
	e.assertConserved(couponID, stock)
	if got := e.mustOrder(42, rid); got.Status != orders.StatusCancelled {
		t.Fatalf("order status = %d", got.Status)
	}
}

// 取消 → 管理员发券 → 不影响秒杀守恒：admin 订单不触碰 Redis 计数，取消它也不能回补计数。
func TestSimulationCancellingAnAdminGrantDoesNotInflateTheSeckillCounter(t *testing.T) {
	e := newEnv(t)
	e.seed(couponID, 5)
	svc := e.orderService()
	grant := func() (int64, error) {
		now := e.clock.Now()
		id, inserted, err := orders.Insert(context.Background(), e.pool, orders.NewOrder{
			ID: 777001, UserID: 42, CouponID: couponID, RequestID: "admin-grant", CouponName: "x", CouponType: 2,
			Source: orders.SourceAdmin, Now: now, ExpireAt: now.Add(time.Hour)})
		if !inserted {
			return 0, errors.New("not inserted")
		}
		return id, err
	}
	id, err := grant()
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.Cancel(context.Background(), 42, id); err != nil || !ok {
		t.Fatalf("Cancel = %v, %v", ok, err)
	}
	if n, _ := e.counter(couponID); n != 5 {
		t.Fatalf("counter = %d, want it untouched at 5", n)
	}
}
