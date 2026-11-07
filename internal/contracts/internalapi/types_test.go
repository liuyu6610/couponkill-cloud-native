package internalapi_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func TestPathsArePinned(t *testing.T) {
	cases := map[string]string{
		internalapi.PathStockDeduct:     "/internal/v1/stock/deduct",
		internalapi.PathStockRelease:    "/internal/v1/stock/release",
		internalapi.PathStockSync:       "/internal/v1/stock/sync",
		internalapi.PathCouponRoute:     "/internal/v1/coupons/:id",
		internalapi.PathOrderGrant:      "/internal/v1/orders/grant",
		internalapi.PathUserRoute:       "/internal/v1/users/:id",
		internalapi.CouponPath(1001):    "/internal/v1/coupons/1001",
		internalapi.UserPath(42):        "/internal/v1/users/42",
		internalapi.HeaderInternalToken: "X-Internal-Token",
		internalapi.HeaderRequestID:     "X-Request-Id",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestRequestWireFormatUsesStringIDs(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"deduct", internalapi.DeductRequest{RequestID: "r1", CouponID: 1001, UserID: 1234567890123456789},
			`{"requestId":"r1","couponId":"1001","userId":"1234567890123456789"}`},
		{"release", internalapi.ReleaseRequest{RequestID: "r1", CouponID: 1001},
			`{"requestId":"r1","couponId":"1001"}`},
		{"sync", internalapi.SyncRequest{CouponID: 1001, Available: 500, Source: "connector"},
			`{"couponId":"1001","available":500,"source":"connector"}`},
		{"sync with mode", internalapi.SyncRequest{CouponID: 1001, Available: 500, Source: "connector", Mode: internalapi.SyncModeSet},
			`{"couponId":"1001","available":500,"source":"connector","mode":"set"}`},
		{"grant", internalapi.GrantRequest{RequestID: "r1", UserID: 7, CouponID: 1001},
			`{"requestId":"r1","userId":"7","couponId":"1001"}`},
		{"deduct response", internalapi.DeductResponse{CouponID: 1001, Bucket: 3, AlreadyDeducted: true},
			`{"couponId":"1001","bucket":3,"alreadyDeducted":true}`},
		{"release response", internalapi.ReleaseResponse{CouponID: 1001, Released: true},
			`{"couponId":"1001","released":true,"alreadyReleased":false}`},
		{"sync response", internalapi.SyncResponse{CouponID: 1001, Available: 500},
			`{"couponId":"1001","available":500}`},
		{"grant response", internalapi.GrantResponse{OrderID: 99, AlreadyGranted: false},
			`{"orderId":"99","alreadyGranted":false}`},
		{"user", internalapi.UserInfo{ID: 7, Username: "demo", Role: "user", Status: 1},
			`{"id":"7","username":"demo","role":"user","status":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.v)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestCouponInfoRoundTrip(t *testing.T) {
	start := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	in := internalapi.CouponInfo{
		ID: 1001, Name: "满100减20", Type: internalapi.CouponTypeSeckill, Status: internalapi.CouponStatusEnabled,
		FaceValue: json.Number("20.00"), MinSpend: json.Number("100.00"), ValidDays: 7, PerUserLimit: 1,
		SeckillStartAt: &start, SeckillEndAt: &end,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"id":"1001"`) || !strings.Contains(string(data), `"faceValue":20.00`) {
		t.Fatalf("unexpected wire format: %s", data)
	}
	var out internalapi.CouponInfo
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != 1001 || !out.SeckillStartAt.Equal(start) || !out.SeckillEndAt.Equal(end) || !out.IsSeckill() || !out.Enabled() {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestCouponInfoWithoutWindowEncodesNull(t *testing.T) {
	data, _ := json.Marshal(internalapi.CouponInfo{ID: 1, Type: internalapi.CouponTypeNormal})
	if !strings.Contains(string(data), `"seckillStartAt":null`) {
		t.Fatalf("missing window must encode as null: %s", data)
	}
}

func TestResponsesIgnoreUnknownFields(t *testing.T) {
	var d internalapi.DeductResponse
	if err := json.Unmarshal([]byte(`{"couponId":"5","bucket":2,"alreadyDeducted":false,"future":"x"}`), &d); err != nil {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}
	if d.CouponID != 5 || d.Bucket != 2 {
		t.Fatalf("decode: %+v", d)
	}
}

func TestValidate(t *testing.T) {
	longSource := strings.Repeat("a", 33)
	cases := []struct {
		name string
		err  error
		ok   bool
	}{
		{"deduct ok", internalapi.DeductRequest{RequestID: "r1", CouponID: 1, UserID: 2}.Validate(), true},
		{"deduct empty request id", internalapi.DeductRequest{CouponID: 1, UserID: 2}.Validate(), false},
		{"deduct bad request id", internalapi.DeductRequest{RequestID: "a b", CouponID: 1, UserID: 2}.Validate(), false},
		{"deduct zero coupon", internalapi.DeductRequest{RequestID: "r", UserID: 2}.Validate(), false},
		{"deduct zero user", internalapi.DeductRequest{RequestID: "r", CouponID: 1}.Validate(), false},
		{"release ok", internalapi.ReleaseRequest{RequestID: "r", CouponID: 1}.Validate(), true},
		{"release empty request id", internalapi.ReleaseRequest{CouponID: 1}.Validate(), false},
		{"release zero coupon", internalapi.ReleaseRequest{RequestID: "r"}.Validate(), false},
		{"sync ok", internalapi.SyncRequest{CouponID: 1, Available: 0, Source: "connector"}.Validate(), true},
		{"sync negative", internalapi.SyncRequest{CouponID: 1, Available: -1, Source: "connector"}.Validate(), false},
		{"sync zero coupon", internalapi.SyncRequest{Available: 1, Source: "connector"}.Validate(), false},
		{"sync empty source", internalapi.SyncRequest{CouponID: 1, Available: 1}.Validate(), false},
		{"sync long source", internalapi.SyncRequest{CouponID: 1, Available: 1, Source: longSource}.Validate(), false},
		{"sync odd source", internalapi.SyncRequest{CouponID: 1, Available: 1, Source: "a b"}.Validate(), false},
		{"sync mode min", internalapi.SyncRequest{CouponID: 1, Available: 1, Source: "connector", Mode: internalapi.SyncModeMin}.Validate(), true},
		{"sync mode set", internalapi.SyncRequest{CouponID: 1, Available: 1, Source: "connector", Mode: internalapi.SyncModeSet}.Validate(), true},
		{"sync unknown mode", internalapi.SyncRequest{CouponID: 1, Available: 1, Source: "connector", Mode: "max"}.Validate(), false},
		{"grant ok", internalapi.GrantRequest{RequestID: "r", UserID: 1, CouponID: 2}.Validate(), true},
		{"grant empty request id", internalapi.GrantRequest{UserID: 1, CouponID: 2}.Validate(), false},
		{"grant zero user", internalapi.GrantRequest{RequestID: "r", CouponID: 2}.Validate(), false},
		{"grant zero coupon", internalapi.GrantRequest{RequestID: "r", UserID: 1}.Validate(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ok {
				if tc.err != nil {
					t.Fatalf("unexpected error: %v", tc.err)
				}
				return
			}
			if !errors.Is(tc.err, apperr.InvalidRequest) {
				t.Fatalf("err = %v, want apperr.InvalidRequest", tc.err)
			}
			if got := apperr.StatusOf(tc.err); got != 400 {
				t.Fatalf("status = %d, want 400", got)
			}
		})
	}
}

// 缺省 mode 必须是「只降不升」：漏传 mode 的调用方（包括旧版本）永远不会把库存抬高而造成超卖。
func TestSyncRequestDefaultModeNeverRaisesStock(t *testing.T) {
	if got := (internalapi.SyncRequest{}).EffectiveMode(); got != internalapi.SyncModeMin {
		t.Fatalf("empty mode must mean %q, got %q", internalapi.SyncModeMin, got)
	}
	if got := (internalapi.SyncRequest{Mode: internalapi.SyncModeSet}).EffectiveMode(); got != internalapi.SyncModeSet {
		t.Fatalf("explicit set must be kept, got %q", got)
	}
}
