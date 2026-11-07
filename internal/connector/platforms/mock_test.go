package platforms

import (
	"context"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
)

func TestMockProductIsDeterministic(t *testing.T) {
	m := NewMock(MockConfig{})
	p, err := m.Product(context.Background(), "sku-9")
	if err != nil {
		t.Fatal(err)
	}
	if p.Platform != domain.Mock || p.ExternalSkuID != "sku-9" || p.Title != "Mock商品-sku-9" {
		t.Fatalf("product = %+v", p)
	}
	if p.Price == nil || p.Price.String() != "99.00" || p.OnSale == nil || !*p.OnSale {
		t.Fatalf("price/onSale = %v/%v", p.Price, p.OnSale)
	}
}

func TestMockStockDefaultsAndSuffixEncoding(t *testing.T) {
	m := NewMock(MockConfig{DefaultStock: 500})
	cases := []struct {
		sku       string
		want      int64
		available bool
	}{
		{"mock-sku", 500, true},
		{"mock-sku:800", 800, true},
		{"mock-sku:0", 0, false},
		{"mock-sku:abc", 500, true}, // 非数字后缀：保持默认
		{":7", 500, true},           // 冒号在开头不算后缀（沿用 Java idx>0）
		{"x:", 500, true},           // 冒号在末尾也不算
	}
	for _, c := range cases {
		s, err := m.Stock(context.Background(), c.sku)
		if err != nil {
			t.Fatalf("%s: %v", c.sku, err)
		}
		if s.StockQty == nil || *s.StockQty != c.want || s.StockAvailable == nil || *s.StockAvailable != c.available {
			t.Errorf("%s: qty=%v available=%v, want %d/%v", c.sku, s.StockQty, s.StockAvailable, c.want, c.available)
		}
	}
}

// 前端契约：金额一律是两位小数的 JSON 数字。
func TestPlatformPricesAreNormalizedToTwoDecimals(t *testing.T) {
	cases := map[string]string{"99": "99.00", "99.5": "99.50", "99.50": "99.50", "0.005": "0.01", "1.234": "1.23", "1.999": "2.00", "": ""}
	for in, want := range cases {
		got, err := price(in)
		if err != nil {
			t.Fatalf("price(%q): %v", in, err)
		}
		switch {
		case want == "" && got != nil:
			t.Errorf("price(%q) = %s, want nil", in, got)
		case want != "" && (got == nil || got.String() != want):
			t.Errorf("price(%q) = %v, want %s", in, got, want)
		}
	}
	for _, bad := range []string{"NaN", "-1", "1e3", "abc", "1.2.3"} {
		if _, err := price(bad); err == nil {
			t.Errorf("price(%q) should be rejected", bad)
		}
	}
}

func TestMockIsAlwaysUp(t *testing.T) {
	h := NewMock(MockConfig{}).Health(context.Background())
	if h.Platform != domain.Mock || h.Status != domain.StatusUp {
		t.Fatalf("health = %+v", h)
	}
}
