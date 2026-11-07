package platforms

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// 拼多多开放平台默认值与接口名。
const (
	defaultPDDServerURL = "https://gw-api.pinduoduo.com/api/router"
	pddInfoMethod       = "pdd.goods.information.get"
)

// PDDConfig 配置拼多多适配器。AppKey（client_id）、AppSecret（client_secret）、AccessToken 缺任何一项，
// 平台即 DISABLED。
type PDDConfig struct {
	// ServerURL 是开放平台网关地址，默认 https://gw-api.pinduoduo.com/api/router。
	ServerURL string
	// AppKey / AppSecret / AccessToken 是拼多多开放平台凭证（环境变量 CK_CONNECTOR_PDD_*）。
	AppKey, AppSecret, AccessToken config.Secret
	// Caller 配置出站 HTTP（超时、重试、熔断）；Name 由适配器设置。
	Caller CallerConfig
	// Now 注入时钟（测试用），默认 time.Now。
	Now func() time.Time
}

// PDD 是拼多多开放平台适配器：用 pdd.goods.information.get 读取商品价格、库存与上下架状态。
// externalSkuId 是商品的 goods_id。签名为 md5(secret + 排序后的 k+v + secret) 的大写十六进制。
type PDD struct {
	cfg    PDDConfig
	caller *Caller
	now    func() time.Time
}

// NewPDD 创建拼多多适配器；缺凭证时也能创建（健康状态为 DISABLED，调用返回 ErrDisabled）。
func NewPDD(cfg PDDConfig) *PDD {
	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultPDDServerURL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Caller.Name = string(domain.PDD)
	return &PDD{cfg: cfg, caller: NewCaller(cfg.Caller), now: cfg.Now}
}

// Platform 实现 PlatformAdapter。
func (p *PDD) Platform() domain.Platform { return domain.PDD }

func (p *PDD) configured() bool {
	return p.cfg.AppKey != "" && p.cfg.AppSecret != "" && p.cfg.AccessToken != ""
}

// Health 实现 PlatformAdapter：缺凭证 DISABLED；熔断器打开 DOWN；否则 UP。不发起外部调用。
func (p *PDD) Health(context.Context) domain.Health {
	return credentialedHealth(domain.PDD, p.configured(),
		"缺少 CK_CONNECTOR_PDD_APP_KEY / CK_CONNECTOR_PDD_APP_SECRET / CK_CONNECTOR_PDD_ACCESS_TOKEN", p.caller)
}

// Product 实现 PlatformAdapter：价格取各 SKU 的最低价（接口单位为分，转换为元）。
func (p *PDD) Product(ctx context.Context, sku string) (domain.ProductSnapshot, error) {
	info, err := p.goodsInfo(ctx, sku)
	if err != nil {
		return domain.ProductSnapshot{}, err
	}
	out := domain.ProductSnapshot{Platform: domain.PDD, ExternalSkuID: sku, Title: "PDD-" + sku}
	if s, ok := asString(info["goods_name"]); ok {
		out.Title = s
	}
	if cents, ok := minSkuPrice(info); ok {
		out.Price = centsToPrice(cents)
	}
	out.OnSale = ptr(onSale(info))
	raw, _ := asString(info["is_onsale"])
	out.RawStatus = raw
	return out, nil
}

// Stock 实现 PlatformAdapter：优先取商品总库存 goods_quantity，没有时对各 SKU 的 quantity 求和；
// 只有在售且数量大于 0 才算有货。
func (p *PDD) Stock(ctx context.Context, sku string) (domain.StockSnapshot, error) {
	info, err := p.goodsInfo(ctx, sku)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	snap := domain.StockSnapshot{Platform: domain.PDD, ExternalSkuID: sku, StockStateDesc: "PDD_GOODS_QUANTITY"}
	if n, ok := asInt64(info["goods_quantity"]); ok && n >= 0 {
		snap.StockQty = &n
	} else if n, ok := sumSkuQuantity(info); ok {
		snap.StockQty = &n
		snap.StockStateDesc = "PDD_SKU_QUANTITY_SUM"
	}
	snap.StockAvailable = ptr(onSale(info) && (snap.StockQty == nil || *snap.StockQty > 0))
	return snap, nil
}

// onSale 判断商品是否在售：is_onsale 为 1（缺失按在售处理，与京东版一致）。
func onSale(info map[string]any) bool {
	if n, ok := asInt64(info["is_onsale"]); ok {
		return n == 1
	}
	return true
}

// minSkuPrice 返回各 SKU 价格（分）中的最小值。
func minSkuPrice(info map[string]any) (int64, bool) {
	skus, _ := info["sku_list"].([]any)
	var best int64
	found := false
	for _, s := range skus {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if cents, ok := asInt64(m["price"]); ok && cents >= 0 && (!found || cents < best) {
			best, found = cents, true
		}
	}
	return best, found
}

// sumSkuQuantity 对各 SKU 的 quantity 求和；没有任何 SKU 数量时返回 false。
func sumSkuQuantity(info map[string]any) (int64, bool) {
	skus, _ := info["sku_list"].([]any)
	var sum int64
	found := false
	for _, s := range skus {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if q, ok := asInt64(m["quantity"]); ok && q >= 0 {
			sum += q
			found = true
		}
	}
	return sum, found
}

// goodsInfo 调用 pdd.goods.information.get 并取出 goods_info 节点。
func (p *PDD) goodsInfo(ctx context.Context, sku string) (map[string]any, error) {
	if !p.configured() {
		return nil, fmt.Errorf("拼多多 Connector 未就绪: %w", ErrDisabled)
	}
	body, err := p.caller.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		params := map[string]string{
			"type":         pddInfoMethod,
			"client_id":    p.cfg.AppKey.Reveal(),
			"access_token": p.cfg.AccessToken.Reveal(),
			"timestamp":    strconv.FormatInt(p.now().Unix(), 10),
			"data_type":    "JSON",
			"version":      "V1",
			"goods_id":     sku,
		}
		params["sign"] = md5Sign(p.cfg.AppSecret.Reveal(), params)
		return newFormRequest(ctx, p.cfg.ServerURL, params)
	})
	if err != nil {
		return nil, err
	}
	root, err := decodeJSON(body)
	if err != nil {
		return nil, err
	}
	if err := errorResponse("PDD", root, "error_code", "error_msg"); err != nil {
		return nil, err
	}
	info, ok := at(root, "goods_info_get_response", "goods_info").(map[string]any)
	if !ok {
		return nil, fmt.Errorf("拼多多未返回商品 %s: %w", sku, ErrNotFound)
	}
	return info, nil
}
