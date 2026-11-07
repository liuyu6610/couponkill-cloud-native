package keys_test

import (
	"strings"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/keys"
)

// 这些字符串是跨服务（order/coupon/gateway）与 Helm 排障脚本共同依赖的契约，
// 任何改动都意味着线上键空间不兼容，必须让测试变红。
func TestKeyStringsArePinned(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"stock", keys.Stock(1001), "ck:stock:1001"},
		{"seckill user marker", keys.SeckillUser(1001, 42), "ck:sk:user:1001:42"},
		{"seckill cooldown", keys.SeckillCooldown(1001, 42), "ck:sk:cool:1001:42"},
		{"seckill request hash", keys.SeckillRequest("r-123"), "ck:sk:req:r-123"},
		{"seckill inflight zset", keys.SeckillInflight(), "ck:sk:inflight"},
		{"coupon meta", keys.CouponMeta(1002), "ck:cpn:meta:1002"},
		{"session", keys.Session("jti-abc"), "ck:sess:jti-abc"},
		{"lock", keys.Lock("seckill-sweeper"), "ck:lock:seckill-sweeper"},
		{"large snowflake id", keys.Stock(1234567890123456789), "ck:stock:1234567890123456789"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
			if !strings.HasPrefix(tc.got, keys.Prefix) {
				t.Fatalf("key %q must start with %q", tc.got, keys.Prefix)
			}
		})
	}
}

func TestPrefixIsCk(t *testing.T) {
	if keys.Prefix != "ck:" {
		t.Fatalf("Prefix = %q, want %q", keys.Prefix, "ck:")
	}
}

// 不同类型、不同参数的键不得相互碰撞：(coupon=1, user=23) 与 (coupon=12, user=3) 必须不同。
func TestKeysDoNotCollide(t *testing.T) {
	a := keys.SeckillUser(1, 23)
	b := keys.SeckillUser(12, 3)
	if a == b {
		t.Fatalf("collision: %q == %q", a, b)
	}
	if keys.SeckillUser(1, 2) == keys.SeckillCooldown(1, 2) {
		t.Fatal("marker and cooldown keys must differ")
	}
}

func TestTTLContract(t *testing.T) {
	if keys.SeckillMarkerTTL != 24*time.Hour {
		t.Errorf("SeckillMarkerTTL = %v, want 24h", keys.SeckillMarkerTTL)
	}
	if keys.SeckillRequestTTL != 24*time.Hour {
		t.Errorf("SeckillRequestTTL = %v, want 24h", keys.SeckillRequestTTL)
	}
	if keys.CouponMetaTTL != 5*time.Minute {
		t.Errorf("CouponMetaTTL = %v, want 5m", keys.CouponMetaTTL)
	}
}

func TestRequestHashFieldNamesArePinned(t *testing.T) {
	want := map[string]string{
		keys.ReqFieldStatus:    "status",
		keys.ReqFieldUserID:    "userId",
		keys.ReqFieldCouponID:  "couponId",
		keys.ReqFieldOrderID:   "orderId",
		keys.ReqFieldReason:    "reason",
		keys.ReqFieldUpdatedAt: "updatedAt",
	}
	for got, w := range want {
		if got != w {
			t.Errorf("field name %q, want %q", got, w)
		}
	}
}
