package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// fakeRepo 是内存版 Repo，语义与 PostgreSQL 实现一致：用户名唯一、TouchLastActive 带条件。
type fakeRepo struct {
	mu       sync.Mutex
	byName   map[string]service.Account
	creates  int
	finds    int
	touches  int
	batches  [][]service.NewAccount
	failNext error
	touchErr error
	now      func() time.Time
}

func newFakeRepo(now func() time.Time) *fakeRepo {
	return &fakeRepo{byName: map[string]service.Account{}, now: now}
}

func (r *fakeRepo) take() error {
	if r.failNext != nil {
		err := r.failNext
		r.failNext = nil
		return err
	}
	return nil
}

func (r *fakeRepo) Create(_ context.Context, a service.NewAccount) (service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creates++
	if err := r.take(); err != nil {
		return service.User{}, err
	}
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

func (r *fakeRepo) FindByUsername(_ context.Context, username string) (service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finds++
	if err := r.take(); err != nil {
		return service.Account{}, err
	}
	acc, ok := r.byName[username]
	if !ok {
		return service.Account{}, service.ErrNotFound
	}
	return acc, nil
}

func (r *fakeRepo) FindByID(_ context.Context, id int64) (service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.take(); err != nil {
		return service.User{}, err
	}
	for _, acc := range r.byName {
		if acc.ID == id {
			return acc.User, nil
		}
	}
	return service.User{}, service.ErrNotFound
}

func (r *fakeRepo) TouchLastActive(_ context.Context, id int64, now time.Time, minInterval time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.touches++
	if r.touchErr != nil {
		return false, r.touchErr
	}
	if err := r.take(); err != nil {
		return false, err
	}
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

func (r *fakeRepo) InsertBatch(_ context.Context, batch []service.NewAccount) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.take(); err != nil {
		return 0, err
	}
	r.batches = append(r.batches, append([]service.NewAccount(nil), batch...))
	n := 0
	for _, a := range batch {
		if _, ok := r.byName[a.Username]; ok {
			continue
		}
		r.byName[a.Username] = service.Account{
			User:         service.User{ID: a.ID, Username: a.Username, Role: a.Role, Status: service.StatusActive},
			PasswordHash: a.PasswordHash,
		}
		n++
	}
	return n, nil
}

func (r *fakeRepo) seed(username, hash, role string, status int, id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName[username] = service.Account{
		User:         service.User{ID: id, Username: username, Role: role, Status: status},
		PasswordHash: hash,
	}
}

func (r *fakeRepo) counts() (creates, finds, touches int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.creates, r.finds, r.touches
}

// fakeHasher 不做 bcrypt：哈希为 "h:"+明文，并统计 Hash/VerifyOrDummy 的调用次数（含 hash 为空的哑比较）。
type fakeHasher struct {
	hashCalls   atomic.Int64
	verifyCalls atomic.Int64
	dummyCalls  atomic.Int64
}

func (h *fakeHasher) Hash(plain string) (string, error) {
	h.hashCalls.Add(1)
	return "h:" + plain, nil
}

func (h *fakeHasher) VerifyOrDummy(hash, plain string) bool {
	h.verifyCalls.Add(1)
	if hash == "" {
		h.dummyCalls.Add(1)
		return false
	}
	return hash == "h:"+plain
}

// fakeSessions 是内存会话表，可注入 Open 失败。
type fakeSessions struct {
	mu      sync.Mutex
	open    map[string]time.Duration
	revoked []string
	openErr error
}

func newFakeSessions() *fakeSessions { return &fakeSessions{open: map[string]time.Duration{}} }

func (s *fakeSessions) Open(_ context.Context, jti string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.openErr != nil {
		return s.openErr
	}
	s.open[jti] = ttl
	return nil
}

func (s *fakeSessions) Revoke(_ context.Context, jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.open, jti)
	s.revoked = append(s.revoked, jti)
	return nil
}

func (s *fakeSessions) active(jti string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.open[jti]
	return ok
}

// seqIDs 返回自增 ID。
type seqIDs struct{ n atomic.Int64 }

func (g *seqIDs) Next() int64 { return g.n.Add(1) + 1000 }

// env 汇集一个被测 Service 及其全部替身。
type env struct {
	svc      *service.Service
	repo     *fakeRepo
	hasher   *fakeHasher
	sessions *fakeSessions
	tokens   *authx.Tokens
	clock    *testkit.Clock
}

func newEnv(t *testing.T) *env {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC))
	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: testkit.JWTSecret, Issuer: "couponkill", Audience: "couponkill-api",
		TTL: time.Hour, Now: clk.Now,
	})
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	e := &env{
		repo: newFakeRepo(clk.Now), hasher: &fakeHasher{}, sessions: newFakeSessions(),
		tokens: tokens, clock: clk,
	}
	e.svc, err = service.New(service.Deps{
		Repo: e.repo, Sessions: e.sessions, Tokens: tokens, Hasher: e.hasher, IDs: &seqIDs{}, Now: clk.Now,
	})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return e
}

// errBoom 是注入的基础设施故障。
var errBoom = errors.New("boom")
