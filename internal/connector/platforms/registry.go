package platforms

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
)

// ErrDown 表示平台当前健康状态为 DOWN（凭证齐全但近期调用持续失败）。
var ErrDown = errors.New("platforms: platform down")

// defaultHealthTTL 是健康状态的缓存时长。
const defaultHealthTTL = 30 * time.Second

// RegistryConfig 配置 Registry。
type RegistryConfig struct {
	// HealthTTL 是健康状态缓存时长，默认 30s；负数表示每次都复查。
	HealthTTL time.Duration
	// Now 注入时钟（测试用），默认 time.Now。
	Now func() time.Time
}

// Registry 按平台持有适配器，并在取用时按缓存的健康状态把关。
//
// 设计要点：所有适配器都注册（缺凭证的也在），健康状态在使用时惰性复查并缓存 HealthTTL，
// 构造时绝不调用 Health——这样启动时 DISABLED/DOWN 的平台一旦恢复就自动参与调度，
// 启动时 UP 之后宕掉的平台也不会一直被路由。只有 UP 才算可用，未知状态一律不可用。并发安全。
type Registry struct {
	ttl     time.Duration
	now     func() time.Time
	entries map[domain.Platform]*entry
	order   []domain.Platform
}

type entry struct {
	adapter PlatformAdapter
	mu      sync.Mutex
	health  domain.Health
	checked time.Time
	valid   bool
}

// NewRegistry 创建注册表。同一平台出现两个适配器是配置错误，立即失败以免静默覆盖。
func NewRegistry(cfg RegistryConfig, adapters ...PlatformAdapter) (*Registry, error) {
	if cfg.HealthTTL == 0 {
		cfg.HealthTTL = defaultHealthTTL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	r := &Registry{ttl: cfg.HealthTTL, now: cfg.Now, entries: make(map[domain.Platform]*entry, len(adapters))}
	for _, a := range adapters {
		p := a.Platform()
		if _, dup := r.entries[p]; dup {
			return nil, fmt.Errorf("platforms: duplicate adapter for platform %s", p)
		}
		r.entries[p] = &entry{adapter: a}
		r.order = append(r.order, p)
	}
	rank := func(p domain.Platform) int {
		if i := slices.Index(domain.AllPlatforms, p); i >= 0 {
			return i
		}
		return len(domain.AllPlatforms)
	}
	slices.SortStableFunc(r.order, func(a, b domain.Platform) int { return rank(a) - rank(b) })
	return r, nil
}

// Get 返回平台的适配器；仅当其健康状态为 UP 时可用。
// 未注册返回 ErrUnknownPlatform；DISABLED 返回 ErrDisabled；DOWN 返回 ErrDown；其它状态也视为不可用。
func (r *Registry) Get(ctx context.Context, p domain.Platform) (PlatformAdapter, error) {
	e, ok := r.entries[p]
	if !ok {
		return nil, fmt.Errorf("%s: %w", p, ErrUnknownPlatform)
	}
	h := r.check(ctx, e)
	switch h.Status {
	case domain.StatusUp:
		return e.adapter, nil
	case domain.StatusDisabled:
		return nil, fmt.Errorf("%s: %s: %w", p, h.Message, ErrDisabled)
	case domain.StatusDown:
		return nil, fmt.Errorf("%s: %s: %w", p, h.Message, ErrDown)
	default:
		return nil, fmt.Errorf("%s: unexpected health status %q: %w", p, h.Status, ErrDown)
	}
}

// Health 返回全部已注册平台的健康状态（展示顺序），使用缓存。
func (r *Registry) Health(ctx context.Context) []domain.Health {
	out := make([]domain.Health, 0, len(r.order))
	for _, p := range r.order {
		out = append(out, r.check(ctx, r.entries[p]))
	}
	return out
}

// check 返回平台的健康状态：缓存未过期直接用，否则惰性复查。同一平台的复查互斥，避免惊群。
func (r *Registry) check(ctx context.Context, e *entry) domain.Health {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.valid && r.ttl > 0 && r.now().Sub(e.checked) < r.ttl {
		return e.health
	}
	h := e.adapter.Health(ctx)
	h.Platform = e.adapter.Platform()
	e.health, e.checked, e.valid = h, r.now(), true
	return h
}
