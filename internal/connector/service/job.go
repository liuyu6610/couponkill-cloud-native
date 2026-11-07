package service

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Scheduler 是 SyncJob 依赖的“执行一轮定时同步”的能力（*SyncService 实现它）。
type Scheduler interface {
	RunScheduled(ctx context.Context) (domain.SyncBatchResult, bool, error)
}

// SyncJob 周期性触发定时库存同步，作为 app.Run 的 worker 运行。
//
// 它只负责“按间隔触发”；互斥（分布式锁）、预算、轮转、续租都在 SyncService 里。
// 单轮的错误、丢锁、甚至 panic 都只影响这一轮：Run 只在 ctx 取消时返回（nil），
// 因为 app.Run 会把 worker 的非 nil 返回值当成致命错误并关停整个服务。
type SyncJob struct {
	scheduler Scheduler
	interval  time.Duration
}

// NewSyncJob 创建定时任务；interval 是两轮之间的间隔（<=0 时取 1 分钟）。
func NewSyncJob(s Scheduler, interval time.Duration) *SyncJob {
	if interval <= 0 {
		interval = time.Minute
	}
	return &SyncJob{scheduler: s, interval: interval}
}

// Run 每隔 interval 执行一轮，直到 ctx 取消。第一轮在一个间隔之后开始，
// 让服务在启动阶段先完成依赖就绪；上一轮没结束时 ticker 会丢弃积压的 tick，不会并发执行两轮。
func (j *SyncJob) Run(ctx context.Context) error {
	logx.From(ctx).Info("stock sync job started", "interval", j.interval.String())
	t := time.NewTicker(j.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			logx.From(ctx).Info("stock sync job stopped")
			return nil
		case <-t.C:
			j.RunOnce(ctx)
		}
	}
}

// RunOnce 执行一轮并吞掉（记录）该轮的错误与 panic。
func (j *SyncJob) RunOnce(ctx context.Context) {
	log := logx.From(ctx)
	defer func() {
		if r := recover(); r != nil {
			log.Error("stock sync round panicked; job continues", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	res, ran, err := j.scheduler.RunScheduled(ctx)
	switch {
	case err != nil:
		log.Warn("stock sync round failed", "err", err)
	case ran && res.SyncedOK > 0:
		log.Info("stock sync round done", "ok", res.SyncedOK, "failed", res.Failed, "skipped", res.Skipped)
	}
}
