package platforms

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
)

// defaultMockStock 是 MockConfig.DefaultStock 的缺省值（沿用 Java 版 500）。
const defaultMockStock = 500

// MockConfig 配置 Mock 适配器。
type MockConfig struct {
	// DefaultStock 是 SKU 不带库存后缀时返回的库存，默认 500。
	DefaultStock int64
}

// Mock 是本地联调用的模拟平台：无需任何凭证，永远 UP，使“绑定→同步→预热→秒杀”可以离线走通。
// SKU 可用 "名字:数量" 的后缀编码库存，例如 "mock-sku:800"。
type Mock struct {
	defaultStock int64
}

// NewMock 创建 Mock 适配器。
func NewMock(cfg MockConfig) *Mock {
	if cfg.DefaultStock <= 0 {
		cfg.DefaultStock = defaultMockStock
	}
	return &Mock{defaultStock: cfg.DefaultStock}
}

// Platform 实现 PlatformAdapter。
func (m *Mock) Platform() domain.Platform { return domain.Mock }

// Product 实现 PlatformAdapter：固定标题与 99.00 的价格。
func (m *Mock) Product(_ context.Context, sku string) (domain.ProductSnapshot, error) {
	p := json.Number("99.00")
	return domain.ProductSnapshot{
		Platform: domain.Mock, ExternalSkuID: sku, Title: "Mock商品-" + sku,
		Price: &p, OnSale: ptr(true), RawStatus: "ON_SALE",
	}, nil
}

// Stock 实现 PlatformAdapter：库存取 SKU 的 ":数量" 后缀，否则用默认库存。
func (m *Mock) Stock(_ context.Context, sku string) (domain.StockSnapshot, error) {
	qty := m.defaultStock
	if idx := strings.LastIndexByte(sku, ':'); idx > 0 && idx < len(sku)-1 {
		if n, err := strconv.ParseInt(sku[idx+1:], 10, 64); err == nil && n >= 0 {
			qty = n
		}
	}
	return domain.StockSnapshot{
		Platform: domain.Mock, ExternalSkuID: sku, StockQty: &qty, StockAvailable: ptr(qty > 0),
		StockStateDesc: "MOCK_IN_STOCK", Area: "mock",
	}, nil
}

// Health 实现 PlatformAdapter：永远 UP。
func (m *Mock) Health(context.Context) domain.Health {
	return domain.Health{Platform: domain.Mock, Status: domain.StatusUp, Message: "mock connector ready"}
}
