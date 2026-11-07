package authx

import (
	"context"
	"sync"
	"time"
)

// Revoker 是服务端会话存储：令牌只有在其 jti 对应的会话存在时才有效，
// 因此登出只需删除这一个 jti（按令牌吊销，重新登录不会让旧令牌复活）。
// 生产实现见 redisx.NewSessionStore（键 ck:sess:{jti}）。
type Revoker interface {
	// Open 登录时登记会话，ttl 为令牌剩余寿命。
	Open(ctx context.Context, jti string, ttl time.Duration) error
	// Revoke 登出时删除会话；jti 不存在不是错误。
	Revoke(ctx context.Context, jti string) error
	// Active 报告会话是否仍然有效。存储不可用时返回错误，由调用方决定 fail-open 还是 fail-closed。
	Active(ctx context.Context, jti string) (bool, error)
}

// MemoryRevoker 是 Revoker 的进程内实现，仅用于测试与单进程场景。
type MemoryRevoker struct {
	mu  sync.Mutex
	exp map[string]time.Time
	now func() time.Time
}

// NewMemoryRevoker 创建内存会话存储。
func NewMemoryRevoker() *MemoryRevoker {
	return &MemoryRevoker{exp: map[string]time.Time{}, now: time.Now}
}

// SetClock 注入时钟（测试用）。
func (m *MemoryRevoker) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// Open 实现 Revoker。
func (m *MemoryRevoker) Open(_ context.Context, jti string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exp[jti] = m.now().Add(ttl)
	return nil
}

// Revoke 实现 Revoker。
func (m *MemoryRevoker) Revoke(_ context.Context, jti string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.exp, jti)
	return nil
}

// Active 实现 Revoker。
func (m *MemoryRevoker) Active(_ context.Context, jti string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.exp[jti]
	if !ok {
		return false, nil
	}
	if !m.now().Before(exp) {
		delete(m.exp, jti)
		return false, nil
	}
	return true, nil
}
