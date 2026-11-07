package platforms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
)

// 响应夹具。Java 版的 TB/PDD 只有“二期预留”的空壳，没有可移植的解析代码，
// 这里的结构依据两家开放平台公开文档的字段命名手写（不是线上抓包），字段假设写在 README。
const (
	tbItemResponse   = `{"item_seller_get_response":{"item":{"num_iid":123456,"title":"淘宝测试商品","price":"128.00","num":57,"approve_status":"onsale"}}}`
	tbItemInstock    = `{"item_seller_get_response":{"item":{"num_iid":123456,"title":"仓库中商品","price":"8.80","num":9,"approve_status":"instock"}}}`
	tbItemMissing    = `{"item_seller_get_response":{}}`
	tbErrorResponse  = `{"error_response":{"code":27,"msg":"Invalid session","sub_code":"isv.session-invalid","sub_msg":"session 已过期"}}`
	pddInfoResponse  = `{"goods_info_get_response":{"goods_info":{"goods_id":9988,"goods_name":"拼多多测试商品","is_onsale":1,"goods_quantity":120,"sku_list":[{"sku_id":1,"price":2990,"quantity":60},{"sku_id":2,"price":1990,"quantity":60}]}}}`
	pddInfoNoTotal   = `{"goods_info_get_response":{"goods_info":{"goods_id":9988,"goods_name":"无总库存字段","is_onsale":0,"sku_list":[{"sku_id":1,"price":1000,"quantity":3},{"sku_id":2,"price":500,"quantity":4}]}}}`
	pddInfoMissing   = `{"goods_info_get_response":{}}`
	pddErrorResponse = `{"error_response":{"error_code":50001,"error_msg":"公共参数错误:access_token","sub_msg":"token 无效"}}`
	signedFakeSecret = "topsecret"
)

// signedFake 是假的开放平台网关：用 md5Sign 校验签名，返回预设响应，并记录收到的参数。
type signedFake struct {
	*httptest.Server
	mu     sync.Mutex
	params []map[string]string
	body   string
	status int
}

func newSignedFake(t *testing.T, body string) *signedFake {
	t.Helper()
	f := &signedFake{body: body, status: http.StatusOK}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		params := map[string]string{}
		for k := range r.PostForm {
			params[k] = r.PostForm.Get(k)
		}
		if got, want := params["sign"], md5Sign(signedFakeSecret, params); got != want {
			t.Errorf("sign = %q, want %q", got, want)
		}
		f.mu.Lock()
		f.params = append(f.params, params)
		status, body := f.status, f.body
		f.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *signedFake) set(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *signedFake) last() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.params[len(f.params)-1]
}

func (f *signedFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.params)
}

func fastCaller(clk *testkit.Clock) CallerConfig {
	return CallerConfig{
		Retries: 1,
		Sleep:   func(context.Context, time.Duration) error { return nil },
		Breaker: limiter.BreakerConfig{FailureThreshold: 2, OpenTimeout: 30 * time.Second, Now: clk.Now},
	}
}

func newTestTB(srv *signedFake, clk *testkit.Clock) *TB {
	return NewTB(TBConfig{
		ServerURL: srv.URL, AppKey: "tb-key", AppSecret: signedFakeSecret, Session: "tb-session",
		Now: clk.Now, Caller: fastCaller(clk),
	})
}

func newTestPDD(srv *signedFake, clk *testkit.Clock) *PDD {
	return NewPDD(PDDConfig{
		ServerURL: srv.URL, AppKey: "pdd-client", AppSecret: signedFakeSecret, AccessToken: "pdd-token",
		Now: clk.Now, Caller: fastCaller(clk),
	})
}

func TestTBRequestShapeAndSignature(t *testing.T) {
	clk := testkit.NewClock(time.Date(2026, 1, 1, 16, 4, 5, 0, time.UTC))
	srv := newSignedFake(t, tbItemResponse)

	if _, err := newTestTB(srv, clk).Product(context.Background(), "123456"); err != nil {
		t.Fatalf("Product: %v", err)
	}
	got := srv.last()
	for k, want := range map[string]string{
		"method": "taobao.item.seller.get", "app_key": "tb-key", "session": "tb-session",
		"format": "json", "v": "2.0", "sign_method": "md5", "num_iid": "123456",
		"fields":    "num_iid,title,price,num,approve_status",
		"timestamp": "2026-01-02 00:04:05",
	} {
		if got[k] != want {
			t.Errorf("param[%s] = %q, want %q", k, got[k], want)
		}
	}
	if !regexp.MustCompile(`^[0-9A-F]{32}$`).MatchString(got["sign"]) {
		t.Errorf("sign = %q", got["sign"])
	}
}

func TestTBProductAndStockParsing(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newSignedFake(t, tbItemResponse)
	tb := newTestTB(srv, clk)

	p, err := tb.Product(context.Background(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if p.Platform != domain.TB || p.Title != "淘宝测试商品" || p.Price == nil || p.Price.String() != "128.00" ||
		p.OnSale == nil || !*p.OnSale || p.RawStatus != "onsale" {
		t.Fatalf("product = %+v", p)
	}
	s, err := tb.Stock(context.Background(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if s.StockQty == nil || *s.StockQty != 57 || s.StockAvailable == nil || !*s.StockAvailable {
		t.Fatalf("stock = %+v", s)
	}

	srv.set(200, tbItemInstock) // 仓库中（未上架）：有数量但不可售
	s, err = tb.Stock(context.Background(), "123456")
	if err != nil || s.StockQty == nil || *s.StockQty != 9 || s.StockAvailable == nil || *s.StockAvailable {
		t.Fatalf("instock stock = %+v, %v; want qty=9 available=false", s, err)
	}
}

func TestTBErrors(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newSignedFake(t, tbItemMissing)
	tb := newTestTB(srv, clk)
	if _, err := tb.Product(context.Background(), "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing item err = %v, want ErrNotFound", err)
	}
	srv.set(200, tbErrorResponse)
	_, err := tb.Stock(context.Background(), "1")
	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Code != "27" {
		t.Fatalf("err = %v, want RejectedError code 27", err)
	}
}

func TestPDDRequestShapeAndSignature(t *testing.T) {
	clk := testkit.NewClock(time.Unix(1767225845, 0))
	srv := newSignedFake(t, pddInfoResponse)

	if _, err := newTestPDD(srv, clk).Product(context.Background(), "9988"); err != nil {
		t.Fatalf("Product: %v", err)
	}
	got := srv.last()
	for k, want := range map[string]string{
		"type": "pdd.goods.information.get", "client_id": "pdd-client", "access_token": "pdd-token",
		"data_type": "JSON", "version": "V1", "goods_id": "9988",
		"timestamp": strconv.FormatInt(1767225845, 10), // 秒级 Unix 时间戳
	} {
		if got[k] != want {
			t.Errorf("param[%s] = %q, want %q", k, got[k], want)
		}
	}
}

func TestPDDProductAndStockParsing(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newSignedFake(t, pddInfoResponse)
	pdd := newTestPDD(srv, clk)

	p, err := pdd.Product(context.Background(), "9988")
	if err != nil {
		t.Fatal(err)
	}
	// 价格取各 SKU 最低价，单位分 -> 元。
	if p.Platform != domain.PDD || p.Title != "拼多多测试商品" || p.Price == nil || p.Price.String() != "19.90" ||
		p.OnSale == nil || !*p.OnSale {
		t.Fatalf("product = %+v", p)
	}
	s, err := pdd.Stock(context.Background(), "9988")
	if err != nil || s.StockQty == nil || *s.StockQty != 120 || s.StockAvailable == nil || !*s.StockAvailable {
		t.Fatalf("stock = %+v, %v", s, err)
	}

	srv.set(200, pddInfoNoTotal) // 没有总库存字段：对各 SKU 数量求和；is_onsale=0 不可售
	s, err = pdd.Stock(context.Background(), "9988")
	if err != nil || s.StockQty == nil || *s.StockQty != 7 || s.StockAvailable == nil || *s.StockAvailable {
		t.Fatalf("sum stock = %+v, %v; want qty=7 available=false", s, err)
	}
}

func TestPDDErrors(t *testing.T) {
	clk := testkit.NewClock(time.Now())
	srv := newSignedFake(t, pddInfoMissing)
	pdd := newTestPDD(srv, clk)
	if _, err := pdd.Product(context.Background(), "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing goods err = %v, want ErrNotFound", err)
	}
	srv.set(200, pddErrorResponse)
	_, err := pdd.Product(context.Background(), "1")
	var rej *RejectedError
	if !errors.As(err, &rej) || rej.Code != "50001" {
		t.Fatalf("err = %v, want RejectedError code 50001", err)
	}
}

func TestTBAndPDDAreDisabledWithoutCredentialsAndNeverCallOut(t *testing.T) {
	srv := newSignedFake(t, tbItemResponse)
	tb := NewTB(TBConfig{ServerURL: srv.URL, AppKey: "k"})
	pdd := NewPDD(PDDConfig{ServerURL: srv.URL, AppKey: "k", AppSecret: "s"}) // 缺 token

	for _, a := range []PlatformAdapter{tb, pdd} {
		if h := a.Health(context.Background()); h.Status != domain.StatusDisabled {
			t.Errorf("%s health = %+v, want DISABLED", a.Platform(), h)
		}
		if _, err := a.Product(context.Background(), "1"); !errors.Is(err, ErrDisabled) {
			t.Errorf("%s Product err = %v, want ErrDisabled", a.Platform(), err)
		}
		if _, err := a.Stock(context.Background(), "1"); !errors.Is(err, ErrDisabled) {
			t.Errorf("%s Stock err = %v, want ErrDisabled", a.Platform(), err)
		}
	}
	if srv.count() != 0 {
		t.Fatal("缺凭证时不得发出任何外部请求")
	}
}

func TestTBAndPDDBreakerLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(*signedFake, *testkit.Clock) PlatformAdapter
		okay string
	}{
		{"TB", func(s *signedFake, c *testkit.Clock) PlatformAdapter { return newTestTB(s, c) }, tbItemResponse},
		{"PDD", func(s *signedFake, c *testkit.Clock) PlatformAdapter { return newTestPDD(s, c) }, pddInfoResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clk := testkit.NewClock(time.Now())
			srv := newSignedFake(t, "upstream down")
			srv.set(http.StatusServiceUnavailable, "upstream down")
			a := tc.make(srv, clk)
			ctx := context.Background()

			for i := 0; i < 2; i++ { // 阈值 2
				if _, err := a.Stock(ctx, "1"); err == nil {
					t.Fatal("want failure")
				}
			}
			if h := a.Health(ctx); h.Status != domain.StatusDown {
				t.Fatalf("health = %+v, want DOWN", h)
			}
			before := srv.count()
			if _, err := a.Stock(ctx, "1"); !errors.Is(err, ErrCircuitOpen) {
				t.Fatalf("err = %v, want ErrCircuitOpen", err)
			}
			if srv.count() != before {
				t.Fatal("熔断打开时必须快速失败，不再请求平台")
			}

			srv.set(200, tc.okay)
			clk.Advance(31 * time.Second)
			if _, err := a.Stock(ctx, "1"); err != nil {
				t.Fatalf("half-open probe: %v", err)
			}
			if h := a.Health(ctx); h.Status != domain.StatusUp {
				t.Fatalf("health = %+v, want UP", h)
			}
		})
	}
}
