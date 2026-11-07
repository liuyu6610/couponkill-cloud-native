package reservation

import (
	"context"
	"testing"
	"time"
)

// legalTransitions 是状态机的独立书面定义（与 status.go 的实现分开写，用来互相校验）。
// 状态图见 README：PENDING→FIRING→QUEUED→SUCCESS/FAILED；PENDING→CANCELLED；
// PENDING 不能直接过期（必须先被认领成 FIRING，由触发时的校验判定活动已结束）；
// FIRING→PENDING（瞬态失败/崩溃回收）、FIRING→EXPIRED（触发时发现活动已结束）、
// FIRING→SUCCESS/FAILED（结果事件先于 QUEUED 回写到达，或触发时即终态失败）。
var legalTransitions = map[[2]Status]bool{
	{StatusPending, StatusFiring}:    true,
	{StatusPending, StatusCancelled}: true,
	{StatusFiring, StatusQueued}:     true,
	{StatusFiring, StatusFailed}:     true,
	{StatusFiring, StatusPending}:    true,
	{StatusFiring, StatusExpired}:    true,
	{StatusFiring, StatusSuccess}:    true,
	{StatusQueued, StatusSuccess}:    true,
	{StatusQueued, StatusFailed}:     true,
}

func TestCanTransitionMatchesWrittenTable(t *testing.T) {
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := legalTransitions[[2]Status{from, to}]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s→%s) = %v, 期望 %v", from, to, got, want)
			}
		}
	}
	if CanTransition("BOGUS", StatusPending) || CanTransition(StatusPending, "BOGUS") {
		t.Error("未知状态不得出现在任何迁移里")
	}
}

func TestStatusClassification(t *testing.T) {
	wantTerminal := map[Status]bool{StatusSuccess: true, StatusFailed: true, StatusCancelled: true, StatusExpired: true}
	wantActive := map[Status]bool{StatusPending: true, StatusFiring: true, StatusQueued: true}
	for _, s := range allStatuses {
		if s.Terminal() != wantTerminal[s] {
			t.Errorf("%s.Terminal() = %v", s, s.Terminal())
		}
		if s.Active() != wantActive[s] {
			t.Errorf("%s.Active() = %v", s, s.Active())
		}
		if !s.Valid() {
			t.Errorf("%s 应是合法状态", s)
		}
		// 终态没有任何出边；非终态至少有一条
		out := 0
		for _, to := range allStatuses {
			if CanTransition(s, to) {
				out++
			}
		}
		if s.Terminal() == (out > 0) {
			t.Errorf("%s: 终态=%v 但出边数=%d", s, s.Terminal(), out)
		}
	}
	if Status("BOGUS").Valid() {
		t.Error("BOGUS 不是合法状态")
	}
}

// TestMoveEnforcesEveryTransitionInDatabase 对 7×7 种 (from,to) 组合真实执行一次迁移：
// 只有合法组合改得动，非法组合必须原样保留行，且迁移也不能产生副作用（retry_count 等）。
func TestMoveEnforcesEveryTransitionInDatabase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := e.clock.Now()
	trigger := now.Add(-time.Minute)

	id := int64(0)
	for _, from := range allStatuses {
		for _, to := range allStatuses {
			id++
			insertRow(t, e.pool, row{id: id, userID: id, couponID: 1, status: from, triggerAt: &trigger})

			got, moved, err := move(ctx, e.pool, change{id: id, to: to, now: now, bumpRetry: true})
			if err != nil {
				t.Fatalf("move %s→%s: %v", from, to, err)
			}
			want := legalTransitions[[2]Status{from, to}]
			if moved != want {
				t.Errorf("move %s→%s moved=%v, 期望 %v", from, to, moved, want)
			}
			stored := load(t, e.pool, id)
			if want {
				if stored.Status != to || stored.RetryCount != 1 || got.Status != to {
					t.Errorf("%s→%s 合法迁移后状态应为 %s 且 retry_count=1: %+v", from, to, to, stored)
				}
			} else if stored.Status != from || stored.RetryCount != 0 {
				t.Errorf("%s→%s 非法迁移不得改动行: %+v", from, to, stored)
			}
		}
	}
}

func TestMoveFenceAndOwnerPredicates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := e.clock.Now()
	trigger := now.Add(-time.Minute)
	insertRow(t, e.pool, row{id: 1, userID: 7, couponID: 1, status: StatusFiring, triggerAt: &trigger, retryCount: 2})

	two, three := 2, 3
	if _, moved, err := move(ctx, e.pool, change{id: 1, to: StatusQueued, now: now, fence: &three}); err != nil || moved {
		t.Fatalf("fence 不匹配不应迁移: moved=%v err=%v", moved, err)
	}
	if _, moved, err := move(ctx, e.pool, change{id: 1, to: StatusQueued, now: now, fence: &two, userID: 8}); err != nil || moved {
		t.Fatalf("属主不匹配不应迁移: moved=%v err=%v", moved, err)
	}
	req := "rid-1"
	got, moved, err := move(ctx, e.pool, change{id: 1, to: StatusQueued, now: now, fence: &two, userID: 7, requestID: &req})
	if err != nil || !moved || got.RequestID == nil || *got.RequestID != "rid-1" {
		t.Fatalf("fence 与属主都匹配应迁移并写入 request_id: moved=%v got=%+v err=%v", moved, got, err)
	}
}
