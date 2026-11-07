package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// PreheatSummary 是批量预热的结果。
type PreheatSummary struct {
	// Total 是参与预热的启用秒杀券数。
	Total int `json:"total"`
	// Preheated 是预热成功的券数。
	Preheated int `json:"preheated"`
	// Failed 是预热失败的券数。
	Failed int `json:"failed"`
}

// PreheatAll 把所有启用中的秒杀券的库存计数器预热到 Redis，并发度受 Config.PreheatConcurrency 约束。
//
// force 为 false（启动时）只写缺失的计数器，不覆盖在途账本——服务重启不会把正在进行的秒杀的预扣计数重置；
// force 为 true（管理员预热）把计数器无条件对齐到 DB 剩余量。任何一张券失败都不会中断其余券，
// 失败汇总在返回的 error 里（同时已逐条记日志）。ctx 取消会让尚未开始的券直接计为失败。
func (s *Service) PreheatAll(ctx context.Context, force bool) (PreheatSummary, error) {
	ids, err := s.repo.EnabledSeckillIDs(ctx)
	if err != nil {
		return PreheatSummary{}, fmt.Errorf("列出启用中的秒杀券: %w", err)
	}
	mode := cacheIfAbsent
	if force {
		mode = cacheOverwrite
	}

	jobs := make(chan int64)
	var (
		wg       sync.WaitGroup
		ok, fail atomic.Int64
		mu       sync.Mutex
		errs     []error
	)
	workers := min(s.cfg.PreheatConcurrency, max(len(ids), 1))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if _, err := s.syncCache(ctx, id, mode); err != nil {
					fail.Add(1)
					s.log.ErrorContext(ctx, "库存预热失败", "coupon_id", id, "err", err)
					mu.Lock()
					errs = append(errs, fmt.Errorf("coupon %d: %w", id, err))
					mu.Unlock()
					continue
				}
				ok.Add(1)
			}
		}()
	}
feed:
	for _, id := range ids {
		select {
		case jobs <- id:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	sum := PreheatSummary{Total: len(ids), Preheated: int(ok.Load()), Failed: int(fail.Load())}
	if skipped := sum.Total - sum.Preheated - sum.Failed; skipped > 0 {
		sum.Failed += skipped
		errs = append(errs, fmt.Errorf("预热被取消，%d 张券未处理: %w", skipped, ctx.Err()))
	}
	s.log.InfoContext(ctx, "库存预热完成", "total", sum.Total, "preheated", sum.Preheated, "failed", sum.Failed, "force", force)
	return sum, errors.Join(errs...)
}

// PreheatOneResult 是单券预热的结果。
type PreheatOneResult struct {
	// CouponID 回显券 ID。
	CouponID int64
	// Stock 是写入计数器的库存（等于 DB 剩余量）。
	Stock int64
}

// PreheatOne 把一张启用中的秒杀券的计数器对齐到 DB 剩余量。券不存在 404；非秒杀券 400（没有计数器可预热）；
// 停用券 409。Redis 失败返回 503。
func (s *Service) PreheatOne(ctx context.Context, id int64) (PreheatOneResult, error) {
	if id <= 0 {
		return PreheatOneResult{}, apperr.InvalidRequest.WithMessage("优惠券ID必须为正数")
	}
	st, err := s.repo.StockState(ctx, id)
	if err != nil {
		return PreheatOneResult{}, err
	}
	if st.Type != TypeSeckill {
		return PreheatOneResult{}, apperr.InvalidRequest.WithMessage("请求参数错误: 仅秒杀券需要预热库存")
	}
	if st.Status != StatusEnabled {
		return PreheatOneResult{}, apperr.CouponDisabled
	}
	used, err := s.syncCache(ctx, id, cacheOverwrite)
	if err != nil {
		s.log.ErrorContext(ctx, "单券库存预热失败", "coupon_id", id, "err", err)
		return PreheatOneResult{}, apperr.New(apperr.CodeSystemBusy, http.StatusServiceUnavailable, "库存预热失败，请稍后重试").WithCause(err)
	}
	s.log.InfoContext(ctx, "单券库存预热完成", "coupon_id", id, "stock", used.Remaining)
	return PreheatOneResult{CouponID: id, Stock: used.Remaining}, nil
}
