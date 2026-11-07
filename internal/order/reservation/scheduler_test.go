package reservation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// testConfig 是测试用调度配置：退避 1s（由假时钟推进），重试 3 次，回收超时 2 分钟。
func testConfig() Config {
	cfg := DefaultConfig()
	cfg.ScanInterval = 5 * time.Millisecond
	cfg.RetryBackoff = time.Second
	cfg.QueuedSyncAfter = time.Minute
	return cfg
}

func newScheduler(t *testing.T, e *env, sk Seckiller, opts ...SchedulerOption) *Scheduler {
	t.Helper()
	s, err := NewScheduler(e.svc, sk, testConfig(), opts...)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	return s
}

// addDue 写入一条已到点（trigger_at 在一秒前）的 PENDING 预约。
func addDue(t *testing.T, e *env, id, userID, couponID int64) {
	t.Helper()
	trigger := e.clock.Now().Add(-time.Second)
	insertRow(t, e.pool, row{id: id, userID: userID, couponID: couponID, status: StatusPending, triggerAt: &trigger})
}

func runOnce(t *testing.T, s *Scheduler) {
	t.Helper()
	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

func TestSchedulerFiresOnlyDuePendingReservations(t *testing.T) {
	e := newEnv(t)
	sk := okSeckiller()
	now := e.clock.Now()
	future := now.Add(time.Hour)
	addDue(t, e, 1, 1, 1001)
	insertRow(t, e.pool, row{id: 2, userID: 2, couponID: 1001, status: StatusPending, triggerAt: &future})
	insertRow(t, e.pool, row{id: 3, userID: 3, couponID: 1001, status: StatusCancelled, triggerAt: &now})

	runOnce(t, newScheduler(t, e, sk))

	got := load(t, e.pool, 1)
	if got.Status != StatusQueued || got.RequestID == nil || *got.RequestID != requestIDFor(1, 1001) || got.FiredAt == nil {
		t.Fatalf("到点的预约应 QUEUED 并记录 request_id/fired_at: %+v", got)
	}
	if st := load(t, e.pool, 2).Status; st != StatusPending {
		t.Fatalf("未到点的预约不得触发, 状态 %s", st)
	}
	if st := load(t, e.pool, 3).Status; st != StatusCancelled {
		t.Fatalf("已取消的预约不得触发, 状态 %s", st)
	}
	if sk.totalCalls() != 1 || sk.callsFor(1, 1001) != 1 {
		t.Fatalf("入口应只被调用一次, 总调用 %d", sk.totalCalls())
	}
	// 再扫一轮：QUEUED 不会被重复触发
	runOnce(t, newScheduler(t, e, sk))
	if sk.totalCalls() != 1 {
		t.Fatalf("重复扫描不得重复调用入口, 总调用 %d", sk.totalCalls())
	}
}

func TestSchedulerMapsEnterErrorsToTerminalStates(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus Status
		wantCode   int
		wantReason string
		wantNote   string
		wantText   string
	}{
		{"库存不足", apperr.OutOfStock, StatusFailed, apperr.CodeOutOfStock, "OUT_OF_STOCK",
			notification.TypeReservationFailed, "券 1001：库存不足"},
		{"券已停用", apperr.CouponDisabled, StatusFailed, apperr.CodeCouponDisabled, "COUPON_DISABLED",
			notification.TypeReservationFailed, "券 1001：优惠券已停用"},
		{"超过领取上限", apperr.CouponLimitExceeded, StatusFailed, apperr.CodeCouponLimitExceeded, "LIMIT_EXCEEDED",
			notification.TypeReservationFailed, "券 1001：已超过领取上限"},
		{"重复秒杀", apperr.RepeatSeckill, StatusFailed, apperr.CodeRepeatSeckill, "REPEAT",
			notification.TypeReservationFailed, "券 1001：" + repeatMessage},
		{"活动已结束", apperr.ActivityEnded, StatusExpired, apperr.CodeActivityEnded, expiredMessage,
			notification.TypeReservationExpired, "券 1001 活动已结束，预约失效"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			addDue(t, e, 1, 7, 1001)
			sk := newSeckiller(func(context.Context, int64, int64, int) (string, error) { return "", tc.err })

			runOnce(t, newScheduler(t, e, sk))

			got := load(t, e.pool, 1)
			if got.Status != tc.wantStatus || got.FailCode == nil || *got.FailCode != tc.wantCode ||
				got.FailReason == nil || *got.FailReason != tc.wantReason || got.RetryCount != 0 {
				t.Fatalf("终态记录不符: %+v", got)
			}
			var typ, content string
			if err := e.pool.QueryRow(context.Background(),
				`SELECT type, content FROM ord.notifications WHERE user_id = 7`).Scan(&typ, &content); err != nil {
				t.Fatalf("应恰好一条通知: %v", err)
			}
			if typ != tc.wantNote || content != tc.wantText {
				t.Fatalf("通知 %s %q, 期望 %s %q", typ, content, tc.wantNote, tc.wantText)
			}
			// 终态不会再被触发
			runOnce(t, newScheduler(t, e, sk))
			if sk.totalCalls() != 1 {
				t.Fatalf("终态后不得再调用入口, 总调用 %d", sk.totalCalls())
			}
		})
	}
}

func TestSchedulerRetriesTransientErrorsWithBackoffThenFails(t *testing.T) {
	for name, enterErr := range map[string]error{
		"冷却中":     apperr.CoolingDown,
		"未预热":     apperr.NotPreheated,
		"尚未开始":    apperr.NotStarted,
		"系统繁忙":    apperr.SystemBusy,
		"网络错误":    errBoom,
		"入口超时":    context.DeadlineExceeded,
		"入口返回空请求": nil, // 受理但没有 requestId，同样按瞬态处理
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			addDue(t, e, 1, 7, 1001)
			sk := newSeckiller(func(context.Context, int64, int64, int) (string, error) { return "", enterErr })
			s := newScheduler(t, e, sk) // MaxRetry=3，退避 1s、2s

			runOnce(t, s) // 第 1 次尝试
			got := load(t, e.pool, 1)
			if got.Status != StatusPending || got.RetryCount != 1 || sk.totalCalls() != 1 {
				t.Fatalf("第 1 次失败应退回 PENDING 且 retry_count=1: %+v calls=%d", got, sk.totalCalls())
			}
			if got.TriggerAt == nil || !got.TriggerAt.Equal(e.clock.Now().Add(time.Second)) {
				t.Fatalf("首次退避应为 1s: trigger_at=%v now=%v", got.TriggerAt, e.clock.Now())
			}

			e.clock.Advance(900 * time.Millisecond) // 退避未到
			runOnce(t, s)
			if sk.totalCalls() != 1 {
				t.Fatalf("退避期内不得重试, 调用 %d", sk.totalCalls())
			}

			e.clock.Advance(200 * time.Millisecond) // 超过 1s
			runOnce(t, s)                           // 第 2 次尝试
			got = load(t, e.pool, 1)
			if got.Status != StatusPending || got.RetryCount != 2 || sk.totalCalls() != 2 {
				t.Fatalf("第 2 次失败应 retry_count=2: %+v calls=%d", got, sk.totalCalls())
			}
			if !got.TriggerAt.Equal(e.clock.Now().Add(2 * time.Second)) {
				t.Fatalf("第二次退避应翻倍为 2s: %v", got.TriggerAt)
			}
			if n := len(notificationsOf(t, e.pool, 7)); n != 0 {
				t.Fatalf("重试期间不得通知用户, 得到 %d 条", n)
			}

			e.clock.Advance(3 * time.Second)
			runOnce(t, s) // 第 3 次尝试 = MaxRetry：置 FAILED
			got = load(t, e.pool, 1)
			if got.Status != StatusFailed || sk.totalCalls() != 3 || got.FailReason == nil || got.FailCode == nil {
				t.Fatalf("达到最大尝试次数应 FAILED: %+v calls=%d", got, sk.totalCalls())
			}
			if *got.FailReason != "SYSTEM_ERROR" {
				t.Fatalf("重试耗尽的原因码应是 events.Reason 词表里的 SYSTEM_ERROR: %q", *got.FailReason)
			}
			var content string
			if err := e.pool.QueryRow(context.Background(), `SELECT content FROM ord.notifications WHERE user_id = 7`).Scan(&content); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(content, retryExhausted) {
				t.Fatalf("通知应标注已达重试上限: %q", content)
			}
			if notes := notificationsOf(t, e.pool, 7); len(notes) != 1 || notes[0] != notification.TypeReservationFailed {
				t.Fatalf("最终失败应通知一次: %v", notes)
			}

			e.clock.Advance(time.Hour)
			runOnce(t, s)
			if sk.totalCalls() != 3 {
				t.Fatalf("FAILED 后不得再调用, 调用 %d", sk.totalCalls())
			}
		})
	}
}

func TestSchedulerTransientThenSuccessKeepsRetryCount(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)
	sk := newSeckiller(func(_ context.Context, u, c int64, nth int) (string, error) {
		if nth == 1 {
			return "", apperr.CoolingDown
		}
		return requestIDFor(u, c), nil
	})
	s := newScheduler(t, e, sk)
	runOnce(t, s)
	e.clock.Advance(2 * time.Second)
	runOnce(t, s)
	got := load(t, e.pool, 1)
	if got.Status != StatusQueued || got.RetryCount != 1 || got.RequestID == nil {
		t.Fatalf("重试成功应 QUEUED 且保留 retry_count=1: %+v", got)
	}
}

func TestSchedulerRecoversPanickingSeckiller(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)
	sk := newSeckiller(func(context.Context, int64, int64, int) (string, error) { panic("入口炸了") })
	runOnce(t, newScheduler(t, e, sk))
	got := load(t, e.pool, 1)
	if got.Status != StatusPending || got.RetryCount != 1 {
		t.Fatalf("入口 panic 应按瞬态失败退避重试: %+v", got)
	}
}

// TestSchedulerFiftyReservationsFiredExactlyOnceByFourInstances 是核心并发不变式：
// 4 个调度器实例同时（用起跑线对齐）抢 50 条到点预约，每条恰好触发一次、恰好 QUEUED。
// 批量 50 让四个实例的第一次认领争抢完全相同的行；批量 7 让认领在多轮里交错。
func TestSchedulerFiftyReservationsFiredExactlyOnceByFourInstances(t *testing.T) {
	for _, batch := range []int{50, 7} {
		t.Run("batch="+itoa(int64(batch)), func(t *testing.T) {
			e := newEnv(t)
			const total, instances = 50, 4
			for i := int64(1); i <= total; i++ {
				addDue(t, e, i, 100+i, 1001)
			}
			sk := newSeckiller(func(_ context.Context, u, c int64, _ int) (string, error) {
				time.Sleep(2 * time.Millisecond) // 放大并发窗口
				return requestIDFor(u, c), nil
			})

			cfg := testConfig()
			cfg.BatchSize = batch
			ctx, cancel := context.WithCancel(context.Background())
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range instances {
				s, err := NewScheduler(e.svc, sk, cfg)
				if err != nil {
					t.Fatal(err)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if err := s.Run(ctx); err != nil {
						t.Errorf("Run 在 ctx 取消时应返回 nil: %v", err)
					}
				}()
			}
			close(start)

			deadline := time.Now().Add(20 * time.Second)
			for {
				var queued int
				if err := e.pool.QueryRow(context.Background(),
					`SELECT count(*) FROM ord.reservations WHERE status = 'QUEUED'`).Scan(&queued); err != nil {
					t.Fatal(err)
				}
				if queued == total {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("20s 内只有 %d/%d 条变为 QUEUED", queued, total)
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
			wg.Wait()

			for i := int64(1); i <= total; i++ {
				if n := sk.callsFor(100+i, 1001); n != 1 {
					t.Errorf("预约 %d 应恰好触发 1 次, 实际 %d 次", i, n)
				}
				if got := load(t, e.pool, i); got.Status != StatusQueued || got.RequestID == nil || got.RetryCount != 0 {
					t.Errorf("预约 %d 状态不符: %+v", i, got)
				}
			}
			if sk.totalCalls() != total {
				t.Errorf("入口总调用 %d, 期望 %d", sk.totalCalls(), total)
			}
		})
	}
}

// TestSchedulerCrashAfterClaimBeforeEnterIsRecoveredAfterStaleTimeout：认领后、入口调用前进程崩溃
// （用 ctx 取消模拟：入口什么都没做，调度器也没有写回任何结果），预约停在 FIRING；
// 超时前不会被抢走，超时后被回收、退避、重新触发。
func TestSchedulerCrashAfterClaimBeforeEnterIsRecoveredAfterStaleTimeout(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)

	crashCtx, crash := context.WithCancel(context.Background())
	crashing := newSeckiller(func(ctx context.Context, _, _ int64, _ int) (string, error) {
		crash() // 进程“死”在入口调用里：没有任何效果，也没有机会写回
		return "", ctx.Err()
	})
	s1 := newScheduler(t, e, crashing)
	if err := s1.RunOnce(crashCtx); err != nil && crashCtx.Err() == nil {
		t.Fatal(err)
	}
	got := load(t, e.pool, 1)
	if got.Status != StatusFiring || got.RetryCount != 0 {
		t.Fatalf("崩溃后预约应停在 FIRING: %+v", got)
	}

	healthy := okSeckiller()
	s2 := newScheduler(t, e, healthy)
	e.clock.Advance(testConfig().StaleFiringTimeout - time.Second) // 还没超时
	runOnce(t, s2)
	if st := load(t, e.pool, 1).Status; st != StatusFiring || healthy.totalCalls() != 0 {
		t.Fatalf("超时之前不得回收他人的认领: %s calls=%d", st, healthy.totalCalls())
	}

	e.clock.Advance(2 * time.Second) // 超时
	runOnce(t, s2)
	got = load(t, e.pool, 1)
	if got.Status != StatusPending || got.RetryCount != 1 || healthy.totalCalls() != 0 {
		t.Fatalf("超时后应回收为 PENDING(retry=1) 并退避: %+v calls=%d", got, healthy.totalCalls())
	}

	e.clock.Advance(2 * time.Second) // 退避结束
	runOnce(t, s2)
	got = load(t, e.pool, 1)
	if got.Status != StatusQueued || got.RequestID == nil || healthy.totalCalls() != 1 {
		t.Fatalf("回收后应重新触发并 QUEUED: %+v calls=%d", got, healthy.totalCalls())
	}
}

// TestSchedulerCrashAfterEnterBeforeWritebackMapsRepeatSeckill：入口已受理（用户已占位）但进程在写回前崩溃，
// 回收后再次触发，入口返回 RepeatSeckill —— 预约终结为 FAILED(10010) 并提示去「我的券」查看，不会重复下单。
func TestSchedulerCrashAfterEnterBeforeWritebackMapsRepeatSeckill(t *testing.T) {
	e := newEnv(t)
	fired := e.clock.Now().Add(-10 * time.Minute) // 认领发生在很久以前
	trigger := fired.Add(-time.Second)
	insertRow(t, e.pool, row{id: 1, userID: 7, couponID: 1001, status: StatusFiring, triggerAt: &trigger, firedAt: &fired})
	sk := newSeckiller(func(context.Context, int64, int64, int) (string, error) { return "", apperr.RepeatSeckill })
	s := newScheduler(t, e, sk)

	runOnce(t, s) // 回收 → PENDING(retry=1, 退避 1s)
	if got := load(t, e.pool, 1); got.Status != StatusPending || got.RetryCount != 1 {
		t.Fatalf("应先回收为 PENDING: %+v", got)
	}
	e.clock.Advance(1100 * time.Millisecond)
	runOnce(t, s)

	got := load(t, e.pool, 1)
	if got.Status != StatusFailed || got.FailCode == nil || *got.FailCode != apperr.CodeRepeatSeckill {
		t.Fatalf("RepeatSeckill 应映射为 FAILED(10010): %+v", got)
	}
	var content string
	if err := e.pool.QueryRow(context.Background(), `SELECT content FROM ord.notifications WHERE user_id = 7`).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if content != "券 1001："+repeatMessage {
		t.Fatalf("通知应引导用户去我的券确认: %q", content)
	}
}

func TestSchedulerStaleRecoveryFailsWhenRetryBudgetIsSpent(t *testing.T) {
	e := newEnv(t)
	fired := e.clock.Now().Add(-10 * time.Minute)
	trigger := fired.Add(-time.Second)
	// retry_count=2：再失败一次就达到 MaxRetry(3)
	insertRow(t, e.pool, row{id: 1, userID: 7, couponID: 1001, status: StatusFiring, triggerAt: &trigger, firedAt: &fired, retryCount: 2})
	sk := okSeckiller()

	runOnce(t, newScheduler(t, e, sk))

	got := load(t, e.pool, 1)
	if got.Status != StatusFailed || got.FailReason == nil || got.FailCode == nil || *got.FailCode != apperr.CodeSysError ||
		*got.FailReason != "TIMEOUT" {
		t.Fatalf("重试额度耗尽的卡住预约应 FAILED(TIMEOUT): %+v", got)
	}
	if notes := notificationsOf(t, e.pool, 7); len(notes) != 1 || notes[0] != notification.TypeReservationFailed {
		t.Fatalf("应通知用户一次: %v", notes)
	}
	if sk.totalCalls() != 0 {
		t.Fatalf("回收失败的预约不应再调用入口, 调用 %d", sk.totalCalls())
	}
}

// gatedSeckiller 返回一个卡在入口里、直到 release 被关闭才以 rid 受理的 Seckiller；entered 在入口被调用时关闭。
func gatedSeckiller(rid string) (sk *fakeSeckiller, entered, release chan struct{}) {
	entered, release = make(chan struct{}), make(chan struct{})
	sk = newSeckiller(func(context.Context, int64, int64, int) (string, error) {
		close(entered)
		<-release
		return rid, nil
	})
	return sk, entered, release
}

// TestSchedulerLateWritebackAfterRecoveryIsFenced：A 实例卡在入口里太久被回收，B 接手并重新认领（新一轮 FIRING）后，
// A 迟到的“受理成功”写回必须落空（fence），不能把 B 的这一轮覆盖成 QUEUED/错误的 request_id。
func TestSchedulerLateWritebackAfterRecoveryIsFenced(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)

	slowA, enteredA, releaseA := gatedSeckiller("rid-from-A")
	doneA := make(chan error, 1)
	go func() { doneA <- newScheduler(t, e, slowA).RunOnce(context.Background()) }()
	<-enteredA

	// A 还卡在入口里；时间推过回收超时，B 回收这条预约，退避结束后 B 重新认领并同样卡在入口里
	e.clock.Advance(testConfig().StaleFiringTimeout + time.Second)
	runOnce(t, newScheduler(t, e, okSeckiller()))
	if got := load(t, e.pool, 1); got.Status != StatusPending || got.RetryCount != 1 {
		t.Fatalf("B 应已回收为 PENDING(retry=1): %+v", got)
	}
	e.clock.Advance(2 * time.Second)
	slowB, enteredB, releaseB := gatedSeckiller("rid-from-B")
	doneB := make(chan error, 1)
	go func() { doneB <- newScheduler(t, e, slowB).RunOnce(context.Background()) }()
	<-enteredB
	if got := load(t, e.pool, 1); got.Status != StatusFiring || got.RetryCount != 1 {
		t.Fatalf("B 应已重新认领: %+v", got)
	}

	close(releaseA)
	if err := <-doneA; err != nil {
		t.Fatalf("A 的 RunOnce 不应报错（迟到写回只是被丢弃）: %v", err)
	}
	if got := load(t, e.pool, 1); got.Status != StatusFiring || got.RequestID != nil {
		t.Fatalf("A 迟到的写回必须被 fence 拦住，B 的这一轮不受影响: %+v", got)
	}

	close(releaseB)
	if err := <-doneB; err != nil {
		t.Fatal(err)
	}
	if got := load(t, e.pool, 1); got.Status != StatusQueued || got.RequestID == nil || *got.RequestID != "rid-from-B" {
		t.Fatalf("B 的写回应生效: %+v", got)
	}
}

// TestSchedulerDoesNotEnterForRowsReclaimedWhileWaitingInBatch：一批里排在后面的预约在内存里等待时被他人回收，
// 轮到它时 touch 的 fence 必须拦住，不得再对已不属于自己的预约调用入口。
func TestSchedulerDoesNotEnterForRowsReclaimedWhileWaitingInBatch(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 101, 1001)
	addDue(t, e, 2, 102, 1001)

	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	slow := newSeckiller(func(_ context.Context, u, c int64, _ int) (string, error) {
		if u == 101 {
			once.Do(func() { close(entered) })
			<-release
		}
		return requestIDFor(u, c), nil
	})
	cfg := testConfig()
	cfg.Concurrency = 1 // 同一批里的第二条必须排队等第一条
	a, err := NewScheduler(e.svc, slow, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.RunOnce(context.Background()) }()
	<-entered

	e.clock.Advance(cfg.StaleFiringTimeout + time.Second)
	runOnce(t, newScheduler(t, e, okSeckiller())) // B 回收两条（退避后才会再触发）
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := slow.callsFor(102, 1001); n != 0 {
		t.Fatalf("已被回收的预约不得再被旧认领方触发, 入口被调用 %d 次", n)
	}
	if got := load(t, e.pool, 2); got.Status != StatusPending || got.RetryCount != 1 {
		t.Fatalf("预约 2 应保持被回收后的 PENDING: %+v", got)
	}
}

// TestSchedulerResultEventBeforeQueuedWritebackWins：结果事件在入口返回之前就已回写（FIRING 兜底匹配），
// 调度器随后的 QUEUED 写回落空，最终是 SUCCESS，且只有一条通知。
func TestSchedulerResultEventBeforeQueuedWritebackWins(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)
	sk := newSeckiller(func(ctx context.Context, u, c int64, _ int) (string, error) {
		rid := requestIDFor(u, c)
		// 入口把消息发给 Kafka，消费者飞快建单，结果事件先于入口返回到达
		err := e.svc.OnSeckillResult(ctx, ResultEvent{RequestID: rid, UserID: u, CouponID: c, Success: true, OrderID: 42})
		if err != nil {
			t.Errorf("OnSeckillResult: %v", err)
		}
		return rid, nil
	})

	runOnce(t, newScheduler(t, e, sk))

	got := load(t, e.pool, 1)
	if got.Status != StatusSuccess || got.OrderID == nil || *got.OrderID != 42 {
		t.Fatalf("应为 SUCCESS: %+v", got)
	}
	if n := len(notificationsOf(t, e.pool, 7)); n != 1 {
		t.Fatalf("应恰好一条通知, 得到 %d", n)
	}
}

func TestSchedulerReconcilesStuckQueuedWithResultReader(t *testing.T) {
	e := newEnv(t)
	old := e.clock.Now().Add(-5 * time.Minute)
	for id, rid := range map[int64]string{1: "rid-ok", 2: "rid-failed", 3: "rid-unknown", 4: "rid-fresh"} {
		updated := old
		if id == 4 {
			updated = e.clock.Now().Add(-10 * time.Second) // 还没到回查时间
		}
		r := rid
		insertRow(t, e.pool, row{id: id, userID: 100 + id, couponID: 1001, status: StatusQueued, requestID: &r, updatedAt: updated})
	}
	var mu sync.Mutex
	asked := map[string]int{}
	reader := readerFunc(func(_ context.Context, rid string) (ResultEvent, bool, error) {
		mu.Lock()
		asked[rid]++
		mu.Unlock()
		switch rid {
		case "rid-ok":
			return ResultEvent{Success: true, OrderID: 9}, true, nil
		case "rid-failed":
			return ResultEvent{Reason: "OUT_OF_STOCK"}, true, nil
		}
		return ResultEvent{}, false, nil
	})
	s := newScheduler(t, e, okSeckiller(), WithResultReader(reader))

	runOnce(t, s)

	if got := load(t, e.pool, 1); got.Status != StatusSuccess || got.OrderID == nil || *got.OrderID != 9 {
		t.Fatalf("回查到成功应回写 SUCCESS: %+v", got)
	}
	if got := load(t, e.pool, 2); got.Status != StatusFailed || got.FailCode == nil || *got.FailCode != apperr.CodeOutOfStock {
		t.Fatalf("回查到失败应回写 FAILED: %+v", got)
	}
	if got := load(t, e.pool, 3); got.Status != StatusQueued {
		t.Fatalf("无结果的预约保持 QUEUED: %+v", got)
	}
	if asked["rid-fresh"] != 0 {
		t.Fatal("刚进入 QUEUED 的预约不应被回查")
	}
	if n := len(notificationsOf(t, e.pool, 101)) + len(notificationsOf(t, e.pool, 102)); n != 2 {
		t.Fatalf("成功与失败各一条通知, 得到 %d", n)
	}

	// 无结果的预约被推迟：同一时刻再扫不会重复回查；过了 QueuedSyncAfter 才会再查
	runOnce(t, s)
	if asked["rid-unknown"] != 1 {
		t.Fatalf("推迟期内不应重复回查, 回查 %d 次", asked["rid-unknown"])
	}
	e.clock.Advance(testConfig().QueuedSyncAfter + time.Second)
	runOnce(t, s)
	if asked["rid-unknown"] != 2 {
		t.Fatalf("到期后应再次回查, 回查 %d 次", asked["rid-unknown"])
	}
}

type readerFunc func(ctx context.Context, requestID string) (ResultEvent, bool, error)

func (f readerFunc) Result(ctx context.Context, requestID string) (ResultEvent, bool, error) {
	return f(ctx, requestID)
}

func TestSchedulerReaderErrorDoesNotBlockOtherReservations(t *testing.T) {
	e := newEnv(t)
	old := e.clock.Now().Add(-5 * time.Minute)
	for id, rid := range map[int64]string{1: "rid-err", 2: "rid-ok"} {
		r := rid
		insertRow(t, e.pool, row{id: id, userID: 100 + id, couponID: 1001, status: StatusQueued, requestID: &r, updatedAt: old})
	}
	reader := readerFunc(func(_ context.Context, rid string) (ResultEvent, bool, error) {
		if rid == "rid-err" {
			return ResultEvent{}, false, errBoom
		}
		return ResultEvent{Success: true, OrderID: 1}, true, nil
	})
	runOnce(t, newScheduler(t, e, okSeckiller(), WithResultReader(reader)))
	if got := load(t, e.pool, 2); got.Status != StatusSuccess {
		t.Fatalf("单条回查失败不应影响其他预约: %+v", got)
	}
	if got := load(t, e.pool, 1); got.Status != StatusQueued {
		t.Fatalf("回查失败的预约应保持 QUEUED: %+v", got)
	}
}

func TestSchedulerRunStopsOnContextCancelAndHonoursDisabled(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)

	cfg := testConfig()
	cfg.Enabled = false
	sk := okSeckiller()
	s, err := NewScheduler(e.svc, sk, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	if sk.totalCalls() != 0 {
		t.Fatal("禁用时不得触发任何预约")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 应在 ctx 取消后返回 nil: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run 在 ctx 取消后没有退出")
	}
}

func TestNewSchedulerRejectsInvalidConfig(t *testing.T) {
	e := newEnv(t)
	bad := testConfig()
	bad.StaleFiringTimeout = bad.EnterTimeout // 回收超时必须大于入口超时
	if _, err := NewScheduler(e.svc, okSeckiller(), bad); err == nil {
		t.Fatal("StaleFiringTimeout <= EnterTimeout 应被拒绝")
	}
	zero := testConfig()
	zero.BatchSize = 0
	if _, err := NewScheduler(e.svc, okSeckiller(), zero); err == nil {
		t.Fatal("BatchSize=0 应被拒绝")
	}
	if _, err := NewScheduler(nil, okSeckiller(), testConfig()); err == nil {
		t.Fatal("缺少 Service 应被拒绝")
	}
	if _, err := NewScheduler(e.svc, nil, testConfig()); err == nil {
		t.Fatal("缺少 Seckiller 应被拒绝")
	}
}

func TestClassifyEnterErrors(t *testing.T) {
	cases := []struct {
		err  error
		want outcome
	}{
		{nil, outcomeQueued},
		{apperr.RepeatSeckill, outcomeRepeat},
		{apperr.ActivityEnded, outcomeExpired},
		{apperr.OutOfStock, outcomeTerminal},
		{apperr.CouponDisabled, outcomeTerminal},
		{apperr.CouponNotFound, outcomeTerminal},
		{apperr.CouponLimitExceeded, outcomeTerminal},
		{apperr.InvalidRequest, outcomeTerminal},
		{apperr.CoolingDown, outcomeRetry},
		{apperr.NotPreheated, outcomeRetry},
		{apperr.NotStarted, outcomeRetry},
		{apperr.RateLimited, outcomeRetry},
		{apperr.SystemBusy, outcomeRetry},
		{apperr.CouponServiceUnavailable, outcomeRetry},
		{apperr.SysError, outcomeRetry},
		{errBoom, outcomeRetry},
		{context.DeadlineExceeded, outcomeRetry},
		{errors.Join(errBoom, apperr.OutOfStock), outcomeTerminal},
	}
	for _, tc := range cases {
		if got := classify(tc.err); got != tc.want {
			t.Errorf("classify(%v) = %d, 期望 %d", tc.err, got, tc.want)
		}
	}
}

func TestRetryDelayDoublesAndCaps(t *testing.T) {
	cfg := testConfig()
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i, w := range want {
		if got := cfg.retryDelay(i); got != w {
			t.Errorf("retryDelay(%d) = %v, 期望 %v", i, got, w)
		}
	}
	if got := cfg.retryDelay(40); got != time.Minute {
		t.Errorf("退避应封顶 1 分钟, 得到 %v", got)
	}
}

func TestTouchFencesOutReclaimedRows(t *testing.T) {
	e := newEnv(t)
	addDue(t, e, 1, 7, 1001)
	ctx := context.Background()
	batch, err := claimDue(ctx, e.pool, e.clock.Now(), 10)
	if err != nil || len(batch) != 1 {
		t.Fatalf("claimDue = %v err=%v", batch, err)
	}
	if ok, err := touch(ctx, e.pool, batch[0], e.clock.Now()); err != nil || !ok {
		t.Fatalf("认领方自己刷新应成功: ok=%v err=%v", ok, err)
	}
	// 另一个实例把它回收并重新认领（retry_count 变了）
	if _, moved, err := move(ctx, e.pool, change{id: 1, to: StatusPending, now: e.clock.Now(), bumpRetry: true}); err != nil || !moved {
		t.Fatalf("回收失败: moved=%v err=%v", moved, err)
	}
	if ok, err := touch(ctx, e.pool, batch[0], e.clock.Now()); err != nil || ok {
		t.Fatalf("被回收后旧认领方的刷新必须失败: ok=%v err=%v", ok, err)
	}
	if again, _ := claimDue(ctx, e.pool, e.clock.Now().Add(time.Hour), 10); len(again) != 1 || again[0].retryCount != 1 {
		t.Fatalf("重新认领应拿到 retry_count=1 的新一轮: %+v", again)
	}
	if ok, _ := touch(ctx, e.pool, batch[0], e.clock.Now()); ok {
		t.Fatal("旧一轮的 fence 对新一轮无效")
	}
}
