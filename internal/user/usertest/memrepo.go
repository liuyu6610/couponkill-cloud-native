// Package usertest 提供 user 服务测试用的内存仓储，语义与 PostgreSQL 实现一致
// （用户名唯一、TouchLastActive 带条件、批量插入跳过重名）。只能被测试代码引用。
package usertest

import (
	"context"
	"sync"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// MemRepo 是并发安全的内存版 service.Repo。
type MemRepo struct {
	mu     sync.Mutex
	byName map[string]service.Account
	now    func() time.Time
}

var _ service.Repo = (*MemRepo)(nil)

// NewMemRepo 创建空仓储；now 用于填充创建时间，传 nil 取 time.Now。
func NewMemRepo(now func() time.Time) *MemRepo {
	if now == nil {
		now = time.Now
	}
	return &MemRepo{byName: map[string]service.Account{}, now: now}
}

// Create 实现 service.Repo。
func (r *MemRepo) Create(_ context.Context, a service.NewAccount) (service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[a.Username]; ok {
		return service.User{}, service.ErrUsernameTaken
	}
	now := r.now()
	acc := service.Account{
		User: service.User{
			ID: a.ID, Username: a.Username, Phone: a.Phone, Email: a.Email, Role: a.Role,
			Status: service.StatusActive, CreatedAt: now, UpdatedAt: now,
		},
		PasswordHash: a.PasswordHash,
	}
	r.byName[a.Username] = acc
	return acc.User, nil
}

// FindByUsername 实现 service.Repo。
func (r *MemRepo) FindByUsername(_ context.Context, username string) (service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.byName[username]
	if !ok {
		return service.Account{}, service.ErrNotFound
	}
	return acc, nil
}

// FindByID 实现 service.Repo。
func (r *MemRepo) FindByID(_ context.Context, id int64) (service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, acc := range r.byName {
		if acc.ID == id {
			return acc.User, nil
		}
	}
	return service.User{}, service.ErrNotFound
}

// TouchLastActive 实现 service.Repo。
func (r *MemRepo) TouchLastActive(_ context.Context, id int64, now time.Time, minInterval time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, acc := range r.byName {
		if acc.ID != id {
			continue
		}
		if acc.LastActiveAt != nil && now.Sub(*acc.LastActiveAt) < minInterval {
			return false, nil
		}
		t := now
		acc.LastActiveAt = &t
		r.byName[name] = acc
		return true, nil
	}
	return false, nil
}

// InsertBatch 实现 service.Repo。
func (r *MemRepo) InsertBatch(_ context.Context, batch []service.NewAccount) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	n := 0
	for _, a := range batch {
		if _, ok := r.byName[a.Username]; ok {
			continue
		}
		r.byName[a.Username] = service.Account{
			User: service.User{
				ID: a.ID, Username: a.Username, Phone: a.Phone, Email: a.Email, Role: a.Role,
				Status: service.StatusActive, CreatedAt: now, UpdatedAt: now,
			},
			PasswordHash: a.PasswordHash,
		}
		n++
	}
	return n, nil
}

// Len 返回已有用户数。
func (r *MemRepo) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byName)
}
