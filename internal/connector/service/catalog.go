package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/platforms"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// 校验上限。
const (
	maxSkuLen         = 128
	maxTitleLen       = 200
	defaultCurrency   = "CNY"
	compareTimeout    = 15 * time.Second
	maxCompareEntries = 1024
)

var (
	currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)
	// manualPriceRE 与 NUMERIC(12,2) 对齐：最多 10 位整数、2 位小数，非负。
	manualPriceRE = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)
)

func invalid(format string, args ...any) error {
	return apperr.InvalidRequest.WithMessage(fmt.Sprintf(format, args...))
}

// BindingRepo 是绑定管理用到的仓储能力。
type BindingRepo interface {
	UpsertBinding(ctx context.Context, cmd domain.BindingCommand, enabled bool) (domain.Binding, error)
	ListBindings(ctx context.Context) ([]domain.Binding, error)
	FindBindingByCoupon(ctx context.Context, couponID int64) (*domain.Binding, error)
}

// BindingService 管理平台 SKU 与券的绑定。
type BindingService struct{ repo BindingRepo }

// NewBindingService 创建绑定服务。
func NewBindingService(repo BindingRepo) *BindingService { return &BindingService{repo: repo} }

// Upsert 校验并创建/更新绑定。同一 (platform, sku) 已存在则更新；一张券只能绑一个 SKU（冲突返回 409）。
// syncEnabled 缺省为 true。
func (s *BindingService) Upsert(ctx context.Context, cmd domain.BindingCommand) (domain.Binding, error) {
	if cmd.Platform == "" {
		return domain.Binding{}, invalid("platform 不能为空")
	}
	if _, err := domain.ParsePlatform(string(cmd.Platform)); err != nil {
		return domain.Binding{}, err
	}
	sku, err := cleanSku(cmd.ExternalSkuID)
	if err != nil {
		return domain.Binding{}, err
	}
	if cmd.CouponID <= 0 {
		return domain.Binding{}, invalid("couponId 必须为正数")
	}
	cmd.ExternalSkuID = sku
	enabled := cmd.SyncEnabled == nil || *cmd.SyncEnabled
	return s.repo.UpsertBinding(ctx, cmd, enabled)
}

// List 返回全部绑定。
func (s *BindingService) List(ctx context.Context) ([]domain.Binding, error) {
	return s.repo.ListBindings(ctx)
}

// ByCoupon 按券取绑定；没有绑定返回 (nil, nil)。
func (s *BindingService) ByCoupon(ctx context.Context, couponID int64) (*domain.Binding, error) {
	if couponID <= 0 {
		return nil, invalid("couponId 必须为正数")
	}
	return s.repo.FindBindingByCoupon(ctx, couponID)
}

func cleanSku(raw string) (string, error) {
	sku := strings.TrimSpace(raw)
	switch {
	case sku == "":
		return "", invalid("externalSkuId 不能为空")
	case len(sku) > maxSkuLen:
		return "", invalid("externalSkuId 不能超过 %d 个字符", maxSkuLen)
	}
	return sku, nil
}

// PriceRepo 是价格映射用到的仓储能力。
type PriceRepo interface {
	BindingRepo
	UpsertPriceMap(ctx context.Context, cmd domain.PriceMapCommand, currency string, enabled bool) (domain.PriceMap, error)
	ListPriceMaps(ctx context.Context, couponID int64, enabledOnly bool) ([]domain.PriceMap, error)
	DeletePriceMap(ctx context.Context, id int64) error
}

// PriceConfig 配置 PriceService。
type PriceConfig struct {
	// CacheTTL 是同品比价结果的缓存时长，默认 30s；负数关闭缓存。
	// 比价接口对匿名用户开放且会触发对第三方平台的探测，缓存防止它被当作放大器。
	CacheTTL time.Duration
	// Now 是时钟（测试注入）。
	Now func() time.Time
}

// PriceService 管理同品比价的手工映射，并提供同品比价。
type PriceService struct {
	repo     PriceRepo
	adapters Adapters
	ttl      time.Duration
	now      func() time.Time

	mu    sync.Mutex
	cache map[int64]cached
}

type cached struct {
	at     time.Time
	result domain.PriceCompareResult
}

// NewPriceService 创建价格服务。
func NewPriceService(repo PriceRepo, adapters Adapters, cfg PriceConfig) *PriceService {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &PriceService{repo: repo, adapters: adapters, ttl: cfg.CacheTTL, now: cfg.Now, cache: map[int64]cached{}}
}

// Upsert 校验并按 (coupon, platform) 创建或更新价格映射。currency 缺省 CNY，enabled 缺省 true。
func (s *PriceService) Upsert(ctx context.Context, cmd domain.PriceMapCommand) (domain.PriceMap, error) {
	if cmd.Platform == "" {
		return domain.PriceMap{}, invalid("platform 不能为空")
	}
	if _, err := domain.ParsePlatform(string(cmd.Platform)); err != nil {
		return domain.PriceMap{}, err
	}
	if cmd.CouponID <= 0 {
		return domain.PriceMap{}, invalid("couponId 必须为正数")
	}
	sku, err := cleanSku(cmd.ExternalSkuID)
	if err != nil {
		return domain.PriceMap{}, err
	}
	cmd.ExternalSkuID = sku
	if cmd.Title != nil {
		t := strings.TrimSpace(*cmd.Title)
		if len([]rune(t)) > maxTitleLen {
			return domain.PriceMap{}, invalid("title 不能超过 %d 个字符", maxTitleLen)
		}
		cmd.Title = &t
	}
	if cmd.ManualPrice != nil && !manualPriceRE.MatchString(cmd.ManualPrice.String()) {
		return domain.PriceMap{}, invalid("manualPrice 必须是非负数，最多两位小数")
	}
	currency := defaultCurrency
	if c := strings.ToUpper(strings.TrimSpace(cmd.Currency)); c != "" {
		if !currencyRE.MatchString(c) {
			return domain.PriceMap{}, invalid("currency 必须是 3 位大写字母代码")
		}
		currency = c
	}
	pm, err := s.repo.UpsertPriceMap(ctx, cmd, currency, cmd.Enabled == nil || *cmd.Enabled)
	if err == nil {
		s.invalidate(int64(cmd.CouponID))
	}
	return pm, err
}

// List 按券列出价格映射（含停用的）。
func (s *PriceService) List(ctx context.Context, couponID int64) ([]domain.PriceMap, error) {
	if couponID <= 0 {
		return nil, invalid("couponId 必须为正数")
	}
	return s.repo.ListPriceMaps(ctx, couponID, false)
}

// Delete 删除价格映射（幂等：不存在也成功）。
func (s *PriceService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalid("id 必须为正数")
	}
	if err := s.repo.DeletePriceMap(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	clear(s.cache) // 只知道映射 id，不知道券：整体失效，代价只是多一次探测
	s.mu.Unlock()
	return nil
}

func (s *PriceService) invalidate(couponID int64) {
	s.mu.Lock()
	delete(s.cache, couponID)
	s.mu.Unlock()
}

// Compare 返回券的同品比价：先探测绑定的主 SKU，再追加启用的手工映射
// （有手工价用 MANUAL，没有则探测）。同一 (平台, SKU) 只出现一次。
// 探测失败只让该行置信度降为 LOW 并带说明，不让整个接口失败。
func (s *PriceService) Compare(ctx context.Context, couponID int64) (domain.PriceCompareResult, error) {
	if couponID <= 0 {
		return domain.PriceCompareResult{}, invalid("couponId 必须为正数")
	}
	if res, ok := s.fromCache(couponID); ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, compareTimeout)
	defer cancel()

	binding, err := s.repo.FindBindingByCoupon(ctx, couponID)
	if err != nil {
		return domain.PriceCompareResult{}, err
	}
	maps, err := s.repo.ListPriceMaps(ctx, couponID, true)
	if err != nil {
		return domain.PriceCompareResult{}, err
	}

	res := domain.PriceCompareResult{CouponID: idgen.ID(couponID), Items: []domain.PriceCompareItem{}}
	seen := map[string]bool{}
	if binding != nil {
		bid := binding.ID
		res.BindingID = &bid
		res.Items = append(res.Items, s.probeItem(ctx, binding.Platform, binding.ExternalSkuID, nil))
		seen[binding.Platform.Key(binding.ExternalSkuID)] = true
	}
	for _, m := range maps {
		if seen[m.Platform.Key(m.ExternalSkuID)] {
			continue
		}
		seen[m.Platform.Key(m.ExternalSkuID)] = true
		res.Items = append(res.Items, s.manualOrProbe(ctx, m))
	}
	s.store(couponID, res)
	return res, nil
}

func (s *PriceService) manualOrProbe(ctx context.Context, m domain.PriceMap) domain.PriceCompareItem {
	if m.ManualPrice != nil {
		return domain.PriceCompareItem{
			Platform: m.Platform, ExternalSkuID: m.ExternalSkuID, Title: m.Title, Price: m.ManualPrice,
			Currency: m.Currency, FetchedAt: s.now().UTC(), Source: "MANUAL", Confidence: "MEDIUM",
		}
	}
	return s.probeItem(ctx, m.Platform, m.ExternalSkuID, m.Title)
}

// probeItem 探测平台商品价格；失败时返回 LOW 置信度的占位行（说明里不含内部细节，细节只进日志）。
func (s *PriceService) probeItem(ctx context.Context, p domain.Platform, sku string, fallbackTitle *string) domain.PriceCompareItem {
	item := domain.PriceCompareItem{
		Platform: p, ExternalSkuID: sku, Title: fallbackTitle, Currency: defaultCurrency,
		FetchedAt: s.now().UTC(), Source: "PROBE",
	}
	adapter, err := s.adapters.Get(ctx, p)
	var snap domain.ProductSnapshot
	if err == nil {
		snap, err = adapter.Product(ctx, sku)
	}
	if err != nil {
		logx.From(ctx).Warn("price-compare probe failed", "platform", string(p), "sku", sku, "err", err)
		msg := "探测失败: " + describePlatformError(err)
		item.Confidence, item.Message = "LOW", &msg
		return item
	}
	if snap.Title != "" {
		t := snap.Title
		item.Title = &t
	}
	item.Price = snap.Price
	switch {
	case snap.Price == nil:
		msg := "probe 无价格"
		item.Confidence, item.Message = "LOW", &msg
	case p == domain.JD:
		item.Confidence = "HIGH"
	default:
		item.Confidence = "MEDIUM"
	}
	return item
}

func (s *PriceService) fromCache(couponID int64) (domain.PriceCompareResult, bool) {
	if s.ttl < 0 {
		return domain.PriceCompareResult{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[couponID]
	if !ok || s.now().Sub(c.at) >= s.ttl {
		return domain.PriceCompareResult{}, false
	}
	return c.result, true
}

func (s *PriceService) store(couponID int64, res domain.PriceCompareResult) {
	if s.ttl < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= maxCompareEntries {
		for k, v := range s.cache {
			if s.now().Sub(v.at) >= s.ttl {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= maxCompareEntries {
			clear(s.cache)
		}
	}
	s.cache[couponID] = cached{at: s.now(), result: res}
}

// ProbeService 提供管理端的“探测平台商品/库存”（不写任何状态）。
type ProbeService struct{ adapters Adapters }

// NewProbeService 创建探测服务。
func NewProbeService(adapters Adapters) *ProbeService { return &ProbeService{adapters: adapters} }

// Health 返回全部平台的健康状态。
func (s *ProbeService) Health(ctx context.Context) []domain.Health { return s.adapters.Health(ctx) }

// Product 探测平台商品。未知平台 400；平台不可用 503；平台拒绝 502；SKU 不存在 404。
func (s *ProbeService) Product(ctx context.Context, platform, sku string) (domain.ProductSnapshot, error) {
	adapter, sku, err := s.resolve(ctx, platform, sku)
	if err != nil {
		return domain.ProductSnapshot{}, err
	}
	snap, err := adapter.Product(ctx, sku)
	if err != nil {
		return domain.ProductSnapshot{}, mapPlatformError(err)
	}
	return snap, nil
}

// Stock 探测平台库存，错误映射同 Product。
func (s *ProbeService) Stock(ctx context.Context, platform, sku string) (domain.StockSnapshot, error) {
	adapter, sku, err := s.resolve(ctx, platform, sku)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	snap, err := adapter.Stock(ctx, sku)
	if err != nil {
		return domain.StockSnapshot{}, mapPlatformError(err)
	}
	return snap, nil
}

func (s *ProbeService) resolve(ctx context.Context, platform, sku string) (platforms.PlatformAdapter, string, error) {
	p, err := domain.ParsePlatform(platform)
	if err != nil {
		return nil, "", err
	}
	sku, err = cleanSku(sku)
	if err != nil {
		return nil, "", err
	}
	adapter, err := s.adapters.Get(ctx, p)
	if err != nil {
		return nil, "", mapPlatformError(err)
	}
	return adapter, sku, nil
}

// mapPlatformError 把适配器错误翻译成对外的业务错误；原始错误只作为 cause 进日志。
func mapPlatformError(err error) error {
	var up *platforms.UpstreamError
	var rej *platforms.RejectedError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, platforms.ErrUnknownPlatform):
		return apperr.InvalidRequest.WithMessage("未知平台").WithCause(err)
	case errors.Is(err, platforms.ErrNotFound):
		return domain.ErrPlatformSkuNotFound.WithCause(err)
	case errors.Is(err, platforms.ErrDisabled):
		return domain.ErrPlatformUnavailable.WithMessage("平台未启用（缺少凭证）").WithCause(err)
	case errors.Is(err, platforms.ErrDown), errors.Is(err, platforms.ErrCircuitOpen):
		return domain.ErrPlatformUnavailable.WithMessage("平台暂时不可用（熔断中），请稍后重试").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return domain.ErrPlatformUnavailable.WithMessage("平台调用超时").WithCause(err)
	case errors.As(err, &up):
		return domain.ErrPlatformUnavailable.WithMessage("平台调用失败").WithCause(err)
	case errors.As(err, &rej), errors.Is(err, platforms.ErrBadResponse):
		return domain.ErrPlatformRejected.WithCause(err)
	}
	return err
}

// describePlatformError 给比价行写面向用户的简短说明（不含内部细节）。
func describePlatformError(err error) string {
	if e, ok := apperr.As(mapPlatformError(err)); ok {
		return e.Message
	}
	return "未知错误"
}
