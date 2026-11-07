package platforms

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
)

// 淘宝开放平台（TOP）默认值与接口名。
const (
	defaultTBServerURL = "https://eco.taobao.com/router/rest"
	tbItemMethod       = "taobao.item.seller.get"
	tbItemFields       = "num_iid,title,price,num,approve_status"
)

// TBConfig 配置淘宝适配器。AppKey、AppSecret、Session 缺任何一项，平台即 DISABLED。
type TBConfig struct {
	// ServerURL 是 TOP 网关地址，默认 https://eco.taobao.com/router/rest。
	ServerURL string
	// AppKey / AppSecret / Session 是淘宝开放平台凭证（环境变量 CK_CONNECTOR_TB_*）。
	AppKey, AppSecret, Session config.Secret
	// Caller 配置出站 HTTP（超时、重试、熔断）；Name 由适配器设置。
	Caller CallerConfig
	// Now 注入时钟（测试用），默认 time.Now。
	Now func() time.Time
}

// TB 是淘宝开放平台（TOP）适配器：用 taobao.item.seller.get 读取商品价格、库存与上下架状态。
// externalSkuId 是商品的 num_iid。签名为 TOP 的 md5 方式。
type TB struct {
	cfg    TBConfig
	caller *Caller
	now    func() time.Time
}

// NewTB 创建淘宝适配器；缺凭证时也能创建（健康状态为 DISABLED，调用返回 ErrDisabled）。
func NewTB(cfg TBConfig) *TB {
	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultTBServerURL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Caller.Name = string(domain.TB)
	return &TB{cfg: cfg, caller: NewCaller(cfg.Caller), now: cfg.Now}
}

// Platform 实现 PlatformAdapter。
func (t *TB) Platform() domain.Platform { return domain.TB }

func (t *TB) configured() bool {
	return t.cfg.AppKey != "" && t.cfg.AppSecret != "" && t.cfg.Session != ""
}

// Health 实现 PlatformAdapter：缺凭证 DISABLED；熔断器打开 DOWN；否则 UP。不发起外部调用。
func (t *TB) Health(context.Context) domain.Health {
	return credentialedHealth(domain.TB, t.configured(),
		"缺少 CK_CONNECTOR_TB_APP_KEY / CK_CONNECTOR_TB_APP_SECRET / CK_CONNECTOR_TB_SESSION", t.caller)
}

// Product 实现 PlatformAdapter。
func (t *TB) Product(ctx context.Context, sku string) (domain.ProductSnapshot, error) {
	item, err := t.item(ctx, sku)
	if err != nil {
		return domain.ProductSnapshot{}, err
	}
	return t.productFrom(item, sku)
}

func (t *TB) productFrom(item map[string]any, sku string) (domain.ProductSnapshot, error) {
	var p = domain.ProductSnapshot{Platform: domain.TB, ExternalSkuID: sku, Title: "TB-" + sku}
	if s, ok := asString(item["title"]); ok {
		p.Title = s
	}
	if text, ok := asString(item["price"]); ok {
		n, err := price(text)
		if err != nil {
			return p, err
		}
		p.Price = n
	}
	status, _ := asString(item["approve_status"])
	p.RawStatus = status
	p.OnSale = ptr(status == "onsale")
	return p, nil
}

// Stock 实现 PlatformAdapter：库存取商品的 num；只有上架（onsale）且数量大于 0 才算有货。
func (t *TB) Stock(ctx context.Context, sku string) (domain.StockSnapshot, error) {
	item, err := t.item(ctx, sku)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	status, _ := asString(item["approve_status"])
	snap := domain.StockSnapshot{Platform: domain.TB, ExternalSkuID: sku, StockStateDesc: "TB_ITEM_NUM", Area: ""}
	if n, ok := asInt64(item["num"]); ok && n >= 0 {
		snap.StockQty = &n
	}
	snap.StockAvailable = ptr(status == "onsale" && (snap.StockQty == nil || *snap.StockQty > 0))
	return snap, nil
}

// item 调用 taobao.item.seller.get 并取出 item 节点。
func (t *TB) item(ctx context.Context, sku string) (map[string]any, error) {
	if !t.configured() {
		return nil, fmt.Errorf("淘宝 Connector 未就绪: %w", ErrDisabled)
	}
	body, err := t.caller.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		params := map[string]string{
			"method":      tbItemMethod,
			"app_key":     t.cfg.AppKey.Reveal(),
			"session":     t.cfg.Session.Reveal(),
			"timestamp":   t.now().In(cst).Format(timestampLayout),
			"format":      "json",
			"v":           "2.0",
			"sign_method": "md5",
			"fields":      tbItemFields,
			"num_iid":     sku,
		}
		params["sign"] = md5Sign(t.cfg.AppSecret.Reveal(), params)
		return newFormRequest(ctx, t.cfg.ServerURL, params)
	})
	if err != nil {
		return nil, err
	}
	root, err := decodeJSON(body)
	if err != nil {
		return nil, err
	}
	if err := errorResponse("TB", root, "code", "msg"); err != nil {
		return nil, err
	}
	item, ok := at(root, "item_seller_get_response", "item").(map[string]any)
	if !ok {
		return nil, fmt.Errorf("淘宝未返回商品 %s: %w", sku, ErrNotFound)
	}
	return item, nil
}
