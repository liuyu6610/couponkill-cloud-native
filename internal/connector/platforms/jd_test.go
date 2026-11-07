package platforms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/config"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// 响应夹具：结构取自 Java 版 JdEcommerceConnector 的解析路径（_responce 是京东文档里的原始拼写）。
const (
	jdSkuListResponse = `{"jingdong_sku_read_searchSkuList_responce":{"page":{"data":[{"skuId":123,"skuName":"京东测试商品","jdPrice":"99.50","status":1,"stockNum":88}]}}}`
	jdSkuListOffSale  = `{"jingdong_sku_read_searchSkuList_response":{"page":{"data":[{"skuId":"123","skuName":"下架商品","jdPrice":"10.00","status":2,"stockNum":0}]}}}`
	jdSkuListEmpty    = `{"jingdong_sku_read_searchSkuList_responce":{"page":{"data":[]}}}`
	jdStockResponse   = `{"jingdong_stock_read_searchSkuStock_responce":{"stockList":[{"skuId":"123","stockNum":42,"stockStateDesc":"现货"}]}}`
	jdStockUnlimited  = `{"stockList":[{"skuId":"123","stockNum":-1}]}`
	jdStockZero       = `{"stockList":[{"sku_id":"123","remainNum":0}]}`
	jdErrorResponse   = `{"error_response":{"code":"19","zh_desc":"无效access_token","en_desc":"invalid token"}}`
)

// jdFake 是假的京东开放平台：校验签名并按 method 返回预设响应。
type jdFake struct {
	*httptest.Server
	t       *testing.T
	secret  string
	mu      sync.Mutex
	methods []string
	forms   []url.Values
	resp    map[string]string // method -> body
	status  int
}

func newJDFake(t *testing.T, secret string, resp map[string]string) *jdFake {
	t.Helper()
	f := &jdFake{t: t, secret: secret, resp: resp, status: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("method/content-type = %s %q, want POST form", r.Method, r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		params := map[string]string{}
		for k := range r.PostForm {
			params[k] = r.PostForm.Get(k)
		}
		if want := md5Sign(secret, params); params["sign"] != want {
			t.Errorf("sign = %q, want %q", params["sign"], want)
		}
		f.mu.Lock()
		f.methods = append(f.methods, params["method"])
		f.forms = append(f.forms, r.PostForm)
		status := f.status
		body := f.resp[params["method"]]
		f.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *jdFake) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.methods...)
}

func newTestJD(t *testing.T, srv *jdFake, clk *testkit.Clock) *JD {
	t.Helper()
	return NewJD(JDConfig{
		ServerURL: srv.URL, AppKey: "the-app-key", AppSecret: config.Secret(srv.secret), AccessToken: "the-token",
		Now: clk.Now,
		Caller: CallerConfig{
			Retries: 1,
			Sleep:   func(context.Context, time.Duration) error { return nil },
			Breaker: limiter.BreakerConfig{FailureThreshold: 2, OpenTimeout: 30 * time.Second, Now: clk.Now},
		},
	})
}

func TestMD5SignGoldenVectors(t *testing.T) {
	// 期望值由独立的 MD5 工具对 "secret"+排序后的 k+v+"secret" 算出（见 Java JdJosClient.sign）。
	got := md5Sign("secret", map[string]string{
		"method": "jingdong.sku.read.searchSkuList", "app_key": "k1", "access_token": "tok",
		"timestamp": "2026-01-02 03:04:05", "sign": "MUST-BE-IGNORED",
	})
	if want := "1DB954320B7E718A543964536E8E6A47"; got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
	if got, want := md5Sign("secret", map[string]string{"title": "你好"}), "3EFD39B7AF59D122E7F7C407BDFBE735"; got != want {
		t.Fatalf("utf-8 sign = %s, want %s", got, want)
	}
}

func TestJDRequestShapeAndSignature(t *testing.T) {
	clk := testkit.NewClock(time.Date(2026, 1, 1, 16, 4, 5, 0, time.UTC)) // 北京时间 2026-01-02 00:04:05
	srv := newJDFake(t, "s3cret", map[string]string{"jingdong.sku.read.searchSkuList": jdSkuListResponse})
	jd := newTestJD(t, srv, clk)

	if _, err := jd.Product(context.Background(), "123"); err != nil {
		t.Fatalf("Product: %v", err)
	}
	form := srv.forms[0]
	for k, want := range map[string]string{
		"method": "jingdong.sku.read.searchSkuList", "app_key": "the-app-key", "access_token": "the-token",
		"format": "json", "v": "2.0", "sign_method": "md5",
		"timestamp":         "2026-01-02 00:04:05", // 签名时间戳必须是北京时间
		"360buy_param_json": `{"skuId":"123","field":"skuId,skuName,jdPrice,status,stockNum"}`,
	} {
		if got := form.Get(k); got != want {
			t.Errorf("form[%s] = %q, want %q", k, got, want)
		}
	}
	if !regexp.MustCompile(`^[0-9A-F]{32}$`).MatchString(form.Get("sign")) {
		t.Errorf("sign = %q, want 32 upper hex", form.Get("sign"))
	}
}

func TestJDProductParsing(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{"jingdong.sku.read.searchSkuList": jdSkuListResponse})
	p, err := newTestJD(t, srv, clk).Product(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if p.Platform != domain.JD || p.ExternalSkuID != "123" || p.Title != "京东测试商品" ||
		p.Price == nil || p.Price.String() != "99.50" || p.OnSale == nil || !*p.OnSale || p.RawStatus != "1" {
		t.Fatalf("product = %+v", p)
	}

	srv.mu.Lock()
	srv.resp["jingdong.sku.read.searchSkuList"] = jdSkuListOffSale
	srv.mu.Unlock()
	p, err = newTestJD(t, srv, clk).Product(context.Background(), "123")
	if err != nil || p.OnSale == nil || *p.OnSale || p.Title != "下架商品" {
		t.Fatalf("off-sale product = %+v, %v", p, err)
	}
}

func TestJDProductMissingSkuAndPlatformError(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{"jingdong.sku.read.searchSkuList": jdSkuListEmpty})
	jd := newTestJD(t, srv, clk)
	if _, err := jd.Product(context.Background(), "123"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	srv.mu.Lock()
	srv.resp["jingdong.sku.read.searchSkuList"] = jdErrorResponse
	srv.mu.Unlock()
	_, err := jd.Product(context.Background(), "123")
	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Code != "19" {
		t.Fatalf("err = %v, want RejectedError code 19", err)
	}
	if jd.Health(context.Background()).Status != domain.StatusUp {
		t.Fatal("平台业务拒绝(error_response)不应让健康状态变 DOWN")
	}
}

func TestJDStockPrimaryPathParsing(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{"jingdong.stock.read.searchSkuStock": jdStockResponse})
	s, err := newTestJD(t, srv, clk).Stock(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if s.StockQty == nil || *s.StockQty != 42 || s.StockAvailable == nil || !*s.StockAvailable ||
		s.StockStateDesc != "现货" || s.Area != "1_72_2799_0" {
		t.Fatalf("stock = %+v", s)
	}
	if got := srv.calls(); len(got) != 1 {
		t.Fatalf("calls = %v, 主路径成功时不应再调降级接口", got)
	}
}

func TestJDStockUnlimitedAndZeroQuantities(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{"jingdong.stock.read.searchSkuStock": jdStockUnlimited})
	jd := newTestJD(t, srv, clk)

	s, err := jd.Stock(context.Background(), "123")
	if err != nil || s.StockQty != nil || s.StockAvailable == nil || !*s.StockAvailable {
		t.Fatalf("unlimited(-1): %+v, %v; want qty=nil available=true", s, err)
	}

	srv.mu.Lock()
	srv.resp["jingdong.stock.read.searchSkuStock"] = jdStockZero
	srv.mu.Unlock()
	s, err = jd.Stock(context.Background(), "123")
	if err != nil || s.StockQty == nil || *s.StockQty != 0 || s.StockAvailable == nil || *s.StockAvailable {
		t.Fatalf("zero: %+v, %v; want qty=0 available=false", s, err)
	}
}

func TestJDStockFallsBackToSkuListWhenStockApiFails(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{
		"jingdong.stock.read.searchSkuStock": jdErrorResponse,
		"jingdong.sku.read.searchSkuList":    jdSkuListResponse,
	})
	s, err := newTestJD(t, srv, clk).Stock(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if s.StockQty == nil || *s.StockQty != 88 || s.StockStateDesc != "JD_FALLBACK_SKU_LIST" ||
		s.StockAvailable == nil || !*s.StockAvailable {
		t.Fatalf("fallback stock = %+v", s)
	}
	if got := srv.calls(); len(got) != 2 {
		t.Fatalf("calls = %v, want stock api + one sku list call (降级只多一次调用)", got)
	}
}

func TestJDStockFallbackOffSaleIsUnavailable(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{
		"jingdong.stock.read.searchSkuStock": `{"stockList":[]}`, // 主路径无节点
		"jingdong.sku.read.searchSkuList":    jdSkuListOffSale,
	})
	s, err := newTestJD(t, srv, clk).Stock(context.Background(), "123")
	if err != nil || s.StockAvailable == nil || *s.StockAvailable || s.StockQty == nil || *s.StockQty != 0 {
		t.Fatalf("stock = %+v, %v; want unavailable qty=0", s, err)
	}
}

func TestJDWithoutCredentialsIsDisabledAndNeverCallsOut(t *testing.T) {
	srv := newJDFake(t, "s", nil)
	jd := NewJD(JDConfig{ServerURL: srv.URL, AppKey: "only-key"}) // 缺 secret 与 token

	h := jd.Health(context.Background())
	if h.Status != domain.StatusDisabled || h.Platform != domain.JD {
		t.Fatalf("health = %+v, want DISABLED", h)
	}
	if _, err := jd.Product(context.Background(), "1"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Product err = %v, want ErrDisabled", err)
	}
	if _, err := jd.Stock(context.Background(), "1"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Stock err = %v, want ErrDisabled", err)
	}
	if len(srv.calls()) != 0 {
		t.Fatal("缺凭证时不得发出任何外部请求")
	}
}

func TestJDServerErrorsOpenBreakerThenFastFailThenRecover(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{"jingdong.sku.read.searchSkuList": jdSkuListResponse})
	srv.status = http.StatusBadGateway
	jd := newTestJD(t, srv, clk) // Retries=1：每次逻辑调用 2 次请求；阈值 2 次逻辑失败

	for i := 0; i < 2; i++ {
		if _, err := jd.Product(context.Background(), "123"); err == nil {
			t.Fatal("want upstream failure")
		}
	}
	if got := len(srv.calls()); got != 4 {
		t.Fatalf("http calls = %d, want 4 (2 次逻辑调用 × (1+1 重试))", got)
	}
	if h := jd.Health(context.Background()); h.Status != domain.StatusDown {
		t.Fatalf("health = %+v, want DOWN while breaker open", h)
	}

	if _, err := jd.Product(context.Background(), "123"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if got := len(srv.calls()); got != 4 {
		t.Fatalf("breaker open must fast-fail: http calls = %d", got)
	}

	srv.mu.Lock()
	srv.status = http.StatusOK
	srv.mu.Unlock()
	clk.Advance(31 * time.Second)
	if _, err := jd.Product(context.Background(), "123"); err != nil {
		t.Fatalf("half-open probe: %v", err)
	}
	if h := jd.Health(context.Background()); h.Status != domain.StatusUp {
		t.Fatalf("health = %+v, want UP after recovery", h)
	}
}

func TestJDMalformedPriceAndBodyAreBadResponses(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newJDFake(t, "s", map[string]string{
		"jingdong.sku.read.searchSkuList": `{"page":{"data":[{"skuId":1,"jdPrice":"NaN"}]}}`,
	})
	jd := newTestJD(t, srv, clk)
	if _, err := jd.Product(context.Background(), "1"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("bad price err = %v, want ErrBadResponse", err)
	}
	srv.mu.Lock()
	srv.resp["jingdong.sku.read.searchSkuList"] = `<html>not json</html>`
	srv.mu.Unlock()
	if _, err := jd.Product(context.Background(), "1"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("html err = %v, want ErrBadResponse", err)
	}
}
