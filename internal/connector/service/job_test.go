package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
)

// fakeScheduled 是 SyncJob 依赖的“一轮定时同步”的替身。
type fakeScheduled struct {
	calls atomic.Int32
	fn    func(n int32) (domain.SyncBatchResult, bool, error)
}

func (f *fakeScheduled) RunScheduled(context.Context) (domain.SyncBatchResult, bool, error) {
	n := f.calls.Add(1)
	if f.fn == nil {
		return domain.SyncBatchResult{}, true, nil
	}
	return f.fn(n)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSyncJobRunsPeriodicallyAndStopsWhenContextIsCancelled(t *testing.T) {
	f := &fakeScheduled{}
	job := service.NewSyncJob(f, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	waitFor(t, "3 rounds", func() bool { return f.calls.Load() >= 3 })
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on cancellation（返回错误会让 app.Run 关停整个服务）", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after ctx cancel")
	}
	n := f.calls.Load()
	time.Sleep(50 * time.Millisecond)
	if f.calls.Load() != n {
		t.Fatal("job kept running after cancel")
	}
}

func TestSyncJobSurvivesRoundErrorsAndPanics(t *testing.T) {
	f := &fakeScheduled{fn: func(n int32) (domain.SyncBatchResult, bool, error) {
		switch n {
		case 1:
			return domain.SyncBatchResult{}, false, errors.New("redis down")
		case 2:
			panic("adapter bug")
		}
		return domain.SyncBatchResult{SyncedOK: 1}, true, nil
	}}
	job := service.NewSyncJob(f, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	waitFor(t, "rounds after the failing ones", func() bool { return f.calls.Load() >= 4 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v: 单轮失败或 panic 不得终止任务", err)
	}
}

func TestSyncJobDrivesRealSyncServiceAndSkipsWhileLockIsHeld(t *testing.T) {
	h := newHarness(t, nil)
	h.bind(1)
	h.mr.Set(lockKey, "manual-sync-in-progress")
	job := service.NewSyncJob(h.svc, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	time.Sleep(80 * time.Millisecond)
	if len(h.adapter.calls()) != 0 {
		t.Fatal("锁被占用期间定时任务必须保持沉默")
	}
	h.mr.Del(lockKey) // 手工同步结束，锁让出
	waitFor(t, "first sync after lock released", func() bool { return len(h.coupon.requests()) >= 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
