package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// fakeRepo 是 service.Repo 的内存替身：只实现业务规则测试需要的最小行为，并记录调用。
type fakeRepo struct {
	mu       sync.Mutex
	coupons  map[int64]service.Coupon
	buckets  map[int64][]int64
	created  []service.NewCoupon
	released []internalapi.ReleaseRequest
	deducts  []internalapi.DeductRequest

	deductErr    error
	releaseErr   error
	stateErr     error
	createErr    error
	setAvailArg  []int64
	setAvailMode []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{coupons: map[int64]service.Coupon{}, buckets: map[int64][]int64{}}
}

func (r *fakeRepo) put(c service.Coupon) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.coupons[c.ID] = c
}

func (r *fakeRepo) Available(context.Context, time.Time, int) ([]service.Coupon, error) {
	return nil, nil
}

func (r *fakeRepo) List(_ context.Context, offset, limit int, enabledOnly bool) ([]service.Coupon, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []service.Coupon
	for _, c := range r.coupons {
		if enabledOnly && !c.Enabled() {
			continue
		}
		all = append(all, c)
	}
	return all, int64(len(all)), nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (service.Coupon, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.coupons[id]
	if !ok {
		return service.Coupon{}, apperr.CouponNotFound
	}
	return c, nil
}

func (r *fakeRepo) Create(_ context.Context, nc service.NewCoupon, buckets []int64) (service.Coupon, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return service.Coupon{}, r.createErr
	}
	r.created = append(r.created, nc)
	r.buckets[nc.ID] = buckets
	var rem int64
	for _, b := range buckets {
		rem += b
	}
	c := service.Coupon{
		ID: nc.ID, Name: nc.Name, Description: nc.Description, Type: nc.Type, Status: service.StatusEnabled,
		FaceValue: nc.FaceValue, MinSpend: nc.MinSpend, ValidDays: nc.ValidDays, PerUserLimit: nc.PerUserLimit,
		TotalStock: nc.TotalStock, Remaining: rem, SeckillStartAt: nc.SeckillStartAt, SeckillEndAt: nc.SeckillEndAt,
	}
	r.coupons[nc.ID] = c
	return c, nil
}

func (r *fakeRepo) UpdateWindow(_ context.Context, id int64, start, end time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.coupons[id]
	if !ok {
		return apperr.CouponNotFound
	}
	c.SeckillStartAt, c.SeckillEndAt = &start, &end
	r.coupons[id] = c
	return nil
}

func (r *fakeRepo) UpdateStatus(_ context.Context, id int64, status int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.coupons[id]
	if !ok {
		return apperr.CouponNotFound
	}
	c.Status = status
	r.coupons[id] = c
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.coupons[id]; !ok {
		return 0, nil
	}
	delete(r.coupons, id)
	return 1, nil
}

func (r *fakeRepo) Deduct(_ context.Context, req internalapi.DeductRequest) (service.DeductResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deducts = append(r.deducts, req)
	if r.deductErr != nil {
		return service.DeductResult{}, r.deductErr
	}
	return service.DeductResult{Bucket: 3}, nil
}

func (r *fakeRepo) Release(_ context.Context, req internalapi.ReleaseRequest) (service.ReleaseResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.released = append(r.released, req)
	if r.releaseErr != nil {
		return service.ReleaseResult{}, r.releaseErr
	}
	return service.ReleaseResult{Released: true}, nil
}

func (r *fakeRepo) SetAvailable(_ context.Context, id int64, available int64, _ int, mode string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.coupons[id]
	if !ok {
		return 0, apperr.CouponNotFound
	}
	r.setAvailArg = append(r.setAvailArg, available)
	r.setAvailMode = append(r.setAvailMode, mode)
	if mode == internalapi.SyncModeMin && available >= c.Remaining {
		return c.Remaining, nil
	}
	c.Remaining = available
	r.coupons[id] = c
	return available, nil
}

func (r *fakeRepo) StockState(_ context.Context, id int64) (service.StockState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stateErr != nil {
		return service.StockState{}, r.stateErr
	}
	c, ok := r.coupons[id]
	if !ok {
		return service.StockState{}, apperr.CouponNotFound
	}
	return service.StockState{Type: c.Type, Status: c.Status, Remaining: c.Remaining}, nil
}

func (r *fakeRepo) EnabledSeckillIDs(context.Context) ([]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []int64
	for id, c := range r.coupons {
		if c.IsSeckill() && c.Enabled() {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// fakeCache 是 service.Cache 的内存替身，可注入失败。
type fakeCache struct {
	mu    sync.Mutex
	stock map[int64]int64
	meta  map[int64]bool
	taken []int64
	err   error
}

func newFakeCache() *fakeCache {
	return &fakeCache{stock: map[int64]int64{}, meta: map[int64]bool{}}
}

func (c *fakeCache) PutStock(_ context.Context, id, remaining int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.stock[id] = remaining
	return nil
}

func (c *fakeCache) PutStockIfAbsent(_ context.Context, id, remaining int64) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return false, c.err
	}
	if _, ok := c.stock[id]; ok {
		return false, nil
	}
	c.stock[id] = remaining
	return true, nil
}

func (c *fakeCache) DropStock(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	delete(c.stock, id)
	return nil
}

func (c *fakeCache) TakeStock(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.taken = append(c.taken, id)
	return nil
}

func (c *fakeCache) DropMeta(_ context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	delete(c.meta, id)
	return nil
}

func (c *fakeCache) stockOf(id int64) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.stock[id]
	return v, ok
}

// fakeOrders 是 internalapi.OrderGranter 的替身。
type fakeOrders struct {
	err   error
	calls atomic.Int32
	resp  internalapi.GrantResponse
	last  internalapi.GrantRequest
}

func (o *fakeOrders) GrantOrder(_ context.Context, req internalapi.GrantRequest) (internalapi.GrantResponse, error) {
	o.calls.Add(1)
	o.last = req
	if o.err != nil {
		return internalapi.GrantResponse{}, o.err
	}
	if o.resp.OrderID == 0 {
		return internalapi.GrantResponse{OrderID: 777}, nil
	}
	return o.resp, nil
}

type seqIDs struct{ n atomic.Int64 }

func (s *seqIDs) Next() int64 { return 9000 + s.n.Add(1) }

type harness struct {
	svc    *service.Service
	repo   *fakeRepo
	cache  *fakeCache
	orders *fakeOrders
}

func newHarness(t *testing.T, cfg service.Config) *harness {
	t.Helper()
	h := &harness{repo: newFakeRepo(), cache: newFakeCache(), orders: &fakeOrders{}}
	svc, err := service.New(service.Deps{
		Repo: h.repo, Cache: h.cache, Orders: h.orders, IDs: &seqIDs{}, Config: cfg,
		Now: func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	h.svc = svc
	return h
}

func ptrTime(t time.Time) *time.Time { return &t }

func ptr[T any](v T) *T { return &v }

var errBoom = errors.New("boom")
