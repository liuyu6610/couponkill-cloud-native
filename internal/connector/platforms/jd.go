package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// 京东 JOS 默认值与接口名（沿用 Java 版）。
const (
	defaultJDServerURL = "https://api.jd.com/routerjson"
	defaultJDArea      = "1_72_2799_0"
	jdSkuListMethod    = "jingdong.sku.read.searchSkuList"
	jdStockMethod      = "jingdong.stock.read.searchSkuStock"
	jdSkuFields        = "skuId,skuName,jdPrice,status,stockNum"
	timestampLayout    = "2006-01-02 15:04:05"
)

// JDConfig 配置京东适配器。三项凭证缺任何一项，平台即 DISABLED。
type JDConfig struct {
	// ServerURL 是 JOS 网关地址，默认 https://api.jd.com/routerjson。
	ServerURL string
	// AppKey / AppSecret / AccessToken 是京东开放平台凭证（环境变量 CK_CONNECTOR_JD_*）。
	AppKey, AppSecret, AccessToken config.Secret
	// DefaultArea 是库存查询的默认收货地区，仅回显在快照里。
	DefaultArea string
	// Caller 配置出站 HTTP（超时、重试、熔断）；Name 由适配器设置。
	Caller CallerConfig
	// Now 注入时钟（测试用），默认 time.Now。
	Now func() time.Time
}

// JD 是京东开放平台（JOS）适配器：不依赖官方 SDK，自己实现 MD5 签名。
type JD struct {
	cfg    JDConfig
	caller *Caller
	now    func() time.Time
}

// NewJD 创建京东适配器；缺凭证时也能创建（健康状态为 DISABLED，调用返回 ErrDisabled）。
func NewJD(cfg JDConfig) *JD {
	if cfg.ServerURL == "" {
		cfg.ServerURL = defaultJDServerURL
	}
	if cfg.DefaultArea == "" {
		cfg.DefaultArea = defaultJDArea
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Caller.Name = string(domain.JD)
	return &JD{cfg: cfg, caller: NewCaller(cfg.Caller), now: cfg.Now}
}

// Platform 实现 PlatformAdapter。
func (j *JD) Platform() domain.Platform { return domain.JD }

// CredentialsConfigured 报告三项凭证是否齐全（供 /platforms 展示，不暴露任何密钥）。
func (j *JD) CredentialsConfigured() bool {
	return j.cfg.AppKey != "" && j.cfg.AppSecret != "" && j.cfg.AccessToken != ""
}

// ServerURL 返回 JOS 网关地址。
func (j *JD) ServerURL() string { return j.cfg.ServerURL }

// DefaultArea 返回默认收货地区。
func (j *JD) DefaultArea() string { return j.cfg.DefaultArea }

// AppKeyMasked 返回脱敏后的 appKey（前 2 后 2，其余 ****；过短全部遮蔽）。
func (j *JD) AppKeyMasked() string { return MaskKey(j.cfg.AppKey.Reveal()) }

// Health 实现 PlatformAdapter：缺凭证 DISABLED；熔断器打开 DOWN；否则 UP。不发起外部调用。
func (j *JD) Health(context.Context) domain.Health {
	return credentialedHealth(domain.JD, j.CredentialsConfigured(),
		"缺少 CK_CONNECTOR_JD_APP_KEY / CK_CONNECTOR_JD_APP_SECRET / CK_CONNECTOR_JD_ACCESS_TOKEN", j.caller)
}

// Product 实现 PlatformAdapter：jingdong.sku.read.searchSkuList。
func (j *JD) Product(ctx context.Context, sku string) (domain.ProductSnapshot, error) {
	root, err := j.skuList(ctx, sku)
	if err != nil {
		return domain.ProductSnapshot{}, err
	}
	return j.productFrom(root, sku)
}

// skuList 调用 searchSkuList 并检查平台错误。
func (j *JD) skuList(ctx context.Context, sku string) (any, error) {
	root, err := j.invoke(ctx, jdSkuListMethod, []kv{{"skuId", sku}, {"field", jdSkuFields}})
	if err != nil {
		return nil, err
	}
	if err := errorResponse("JD", root, "code", "zh_desc"); err != nil {
		return nil, err
	}
	return root, nil
}

// skuNode 在 searchSkuList 响应里取第一个 SKU（兼容京东的多种包装拼写）。
func skuNode(root any) map[string]any {
	return firstOf(root,
		[]string{"jingdong_sku_read_searchSkuList_responce", "page", "data"},
		[]string{"jingdong_sku_read_searchSkuList_response", "page", "data"},
		[]string{"page", "data"},
		[]string{"data"},
	)
}

func (j *JD) productFrom(root any, sku string) (domain.ProductSnapshot, error) {
	node := skuNode(root)
	if node == nil {
		return domain.ProductSnapshot{}, fmt.Errorf("京东未返回 SKU %s: %w", sku, ErrNotFound)
	}
	var p *json.Number
	if text, ok := asString(node["jdPrice"]); ok {
		n, err := price(text)
		if err != nil {
			return domain.ProductSnapshot{}, err
		}
		p = n
	}
	onSale := true
	rawStatus := ""
	if v, ok := node["status"]; ok && v != nil {
		rawStatus, _ = asString(v)
		if n, ok := asInt64(v); ok {
			onSale = n == 1
		}
	}
	title := "JD-" + sku
	if s, ok := asString(node["skuName"]); ok {
		title = s
	}
	return domain.ProductSnapshot{Platform: domain.JD, ExternalSkuID: sku, Title: title, Price: p, OnSale: &onSale, RawStatus: rawStatus}, nil
}

// Stock 实现 PlatformAdapter：优先 jingdong.stock.read.searchSkuStock；
// 该接口失败或没有该 SKU 的节点时，降级为 searchSkuList 里的 stockNum（只多一次调用）。
func (j *JD) Stock(ctx context.Context, sku string) (domain.StockSnapshot, error) {
	snap, found, err := j.stockPrimary(ctx, sku)
	switch {
	case err == nil && found:
		return snap, nil
	case err != nil && isFatalForFallback(ctx, err):
		return domain.StockSnapshot{}, err
	case err != nil:
		logx.From(ctx).Warn("searchSkuStock 失败，降级 searchSkuList", "sku", sku, "err", err)
	default:
		logx.From(ctx).Warn("searchSkuStock 无精确节点，降级 searchSkuList", "sku", sku)
	}
	return j.stockFallback(ctx, sku)
}

// isFatalForFallback 判断主路径错误是否不值得再试降级接口：凭证缺失、熔断器打开、调用方放弃。
func isFatalForFallback(ctx context.Context, err error) bool {
	return errors.Is(err, ErrDisabled) || errors.Is(err, ErrCircuitOpen) || ctx.Err() != nil
}

// stockPrimary 调用 searchSkuStock；found=false 表示响应里没有可用节点。
func (j *JD) stockPrimary(ctx context.Context, sku string) (snap domain.StockSnapshot, found bool, err error) {
	root, err := j.invoke(ctx, jdStockMethod, []kv{{"skuIds", sku}})
	if err != nil {
		return snap, false, err
	}
	if err := errorResponse("JD", root, "code", "zh_desc"); err != nil {
		return snap, false, err
	}
	node := stockNode(root, sku)
	if node == nil {
		return snap, false, nil
	}
	var qty *int64
	if n, ok := asInt64(node["stockNum"]); ok {
		qty = &n
	} else if n, ok := asInt64(node["remainNum"]); ok {
		qty = &n
	}
	available := qty == nil || *qty > 0 || *qty == -1
	if qty != nil && *qty < 0 {
		qty = nil
	}
	desc := "JD_STOCK"
	if s, ok := asString(node["stockStateDesc"]); ok {
		desc = s
	}
	return domain.StockSnapshot{
		Platform: domain.JD, ExternalSkuID: sku, StockQty: qty, StockAvailable: &available,
		StockStateDesc: desc, Area: j.cfg.DefaultArea,
	}, true, nil
}

// stockNode 在 searchSkuStock 响应里按 skuId 找节点；找不到时退回列表第一个（与 Java 版一致）。
func stockNode(root any, sku string) map[string]any {
	for _, path := range [][]string{
		{"jingdong_stock_read_searchSkuStock_responce", "stockList"},
		{"jingdong_stock_read_searchSkuStock_response", "stockList"},
		{"stockList"},
		{"result"},
	} {
		arr, ok := at(root, path...).([]any)
		if !ok {
			continue
		}
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := asString(m["skuId"]); id == sku {
				return m
			}
			if id, _ := asString(m["sku_id"]); id == sku {
				return m
			}
		}
		if len(arr) > 0 {
			if m, ok := arr[0].(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

// stockFallback 用一次 searchSkuList 同时得到在售状态与 stockNum。
// 与 Java 版的差异：只调一次（Java 为取库存重复调用一遍）；stockNum 为 null 或负数按“未知”处理，
// 不会被当作 0 而把库存清零。
func (j *JD) stockFallback(ctx context.Context, sku string) (domain.StockSnapshot, error) {
	root, err := j.skuList(ctx, sku)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	product, err := j.productFrom(root, sku)
	if err != nil {
		return domain.StockSnapshot{}, err
	}
	var qty *int64
	if n, ok := asInt64(skuNode(root)["stockNum"]); ok && n >= 0 {
		qty = &n
	}
	onSale := product.OnSale != nil && *product.OnSale
	available := onSale && (qty == nil || *qty > 0)
	return domain.StockSnapshot{
		Platform: domain.JD, ExternalSkuID: sku, StockQty: qty, StockAvailable: &available,
		StockStateDesc: "JD_FALLBACK_SKU_LIST", Area: j.cfg.DefaultArea,
	}, nil
}

// invoke 发出一次已签名的 JOS 调用并解析响应树。
func (j *JD) invoke(ctx context.Context, method string, biz []kv) (any, error) {
	if !j.CredentialsConfigured() {
		return nil, fmt.Errorf("京东 Connector 未就绪: %w", ErrDisabled)
	}
	paramJSON := jsonObject(biz)
	body, err := j.caller.Do(ctx, func(ctx context.Context) (*http.Request, error) {
		params := map[string]string{
			"method":            method,
			"app_key":           j.cfg.AppKey.Reveal(),
			"access_token":      j.cfg.AccessToken.Reveal(),
			"timestamp":         j.now().In(cst).Format(timestampLayout), // 京东要求北京时间，UTC 机器也不会签名失败
			"format":            "json",
			"v":                 "2.0",
			"sign_method":       "md5",
			"360buy_param_json": paramJSON,
		}
		params["sign"] = md5Sign(j.cfg.AppSecret.Reveal(), params)
		return newFormRequest(ctx, j.cfg.ServerURL, params)
	})
	if err != nil {
		return nil, err
	}
	return decodeJSON(body)
}

// newFormRequest 构造 application/x-www-form-urlencoded 的 POST 请求。
func newFormRequest(ctx context.Context, endpoint string, params map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(formValues(params).Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

// MaskKey 对密钥做展示用脱敏：空串返回空；不超过 4 位返回 ****；否则保留前 2 后 2。
func MaskKey(key string) string {
	switch {
	case key == "":
		return ""
	case len(key) <= 4:
		return "****"
	}
	return key[:2] + "****" + key[len(key)-2:]
}
