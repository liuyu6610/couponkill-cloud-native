package gateway_test

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func TestRateLimit_PerIPWithFakeClock(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "3"})
	ipA, ipB := remote("198.51.100.1:1000"), remote("198.51.100.2:1000")

	for i := 0; i < 3; i++ {
		if res := e.do("GET", "/api/v1/coupon/list", ipA); res.ResponseRecorder.Code != 200 {
			t.Fatalf("request %d within the burst: status %d", i, res.ResponseRecorder.Code)
		}
	}
	res := e.do("GET", "/api/v1/coupon/list", ipA)
	if res.ResponseRecorder.Code != http.StatusTooManyRequests || res.Code != apperr.CodeRateLimited {
		t.Fatalf("4th request: status/code = %d/%d, want 429/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeRateLimited)
	}
	if ra, err := strconv.Atoi(res.Header().Get("Retry-After")); err != nil || ra < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", res.Header().Get("Retry-After"))
	}
	if n := e.coupon.Count(); n != 3 {
		t.Errorf("upstream saw %d requests, want 3 (the limited one must not be forwarded)", n)
	}

	// 其他 IP 有自己的桶。
	if res := e.do("GET", "/api/v1/coupon/list", ipB); res.ResponseRecorder.Code != 200 {
		t.Errorf("other IP: status %d, want 200", res.ResponseRecorder.Code)
	}
	// 时间推进后令牌补充。
	e.clock.Advance(1100 * time.Millisecond)
	if res := e.do("GET", "/api/v1/coupon/list", ipA); res.ResponseRecorder.Code != 200 {
		t.Errorf("after refill: status %d, want 200", res.ResponseRecorder.Code)
	}
	if res := e.do("GET", "/api/v1/coupon/list", ipA); res.ResponseRecorder.Code != 429 {
		t.Errorf("only one token was refilled, got status %d", res.ResponseRecorder.Code)
	}
}

// 不变式：时钟静止时，无论多少并发请求，放行数恰好等于突发容量，被拒的绝不转发给上游。
func TestRateLimit_ConcurrentRequestsNeverExceedTheBurst(t *testing.T) {
	const burst, total = 7, 200
	e := newEnv(t, map[string]string{"CK_GATEWAY_SECKILL_RPS": "1", "CK_GATEWAY_SECKILL_BURST": strconv.Itoa(burst)})
	token, _ := e.login(1, "user")

	var allowed, limited atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch e.do("POST", "/api/v1/order/seckill", bearer(token)).ResponseRecorder.Code {
			case 200:
				allowed.Add(1)
			case 429:
				limited.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != burst || limited.Load() != total-burst {
		t.Fatalf("allowed=%d limited=%d, want exactly %d allowed", allowed.Load(), limited.Load(), burst)
	}
	if n := e.order.Count(); n != burst {
		t.Fatalf("upstream saw %d requests, want %d", n, burst)
	}
}

func TestRateLimit_IPLimitAppliesBeforeAuthentication(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "2"})
	for i := 0; i < 2; i++ {
		e.do("GET", "/api/v1/order/user/me", bearer("garbage"))
	}
	res := e.do("GET", "/api/v1/order/user/me", bearer("garbage"))
	if res.ResponseRecorder.Code != 429 {
		t.Fatalf("status = %d, want 429: unauthenticated floods must be rate limited too", res.ResponseRecorder.Code)
	}
}

func TestRateLimit_PerUser(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_USER_RPS": "1", "CK_GATEWAY_USER_BURST": "2"})
	alice, _ := e.login(1, "user")
	bob, _ := e.login(2, "user")

	for i := 0; i < 2; i++ {
		if res := e.do("GET", "/api/v1/order/user/me", bearer(alice)); res.ResponseRecorder.Code != 200 {
			t.Fatalf("alice request %d: status %d", i, res.ResponseRecorder.Code)
		}
	}
	res := e.do("GET", "/api/v1/order/user/me", bearer(alice))
	if res.ResponseRecorder.Code != 429 || res.Header().Get("Retry-After") == "" {
		t.Fatalf("alice 3rd: status %d retry-after %q, want 429 with Retry-After", res.ResponseRecorder.Code, res.Header().Get("Retry-After"))
	}
	// 同一个 IP 上的另一个用户不受影响：用户桶独立于 IP 桶。
	if res := e.do("GET", "/api/v1/order/user/me", bearer(bob)); res.ResponseRecorder.Code != 200 {
		t.Errorf("bob: status %d, want 200", res.ResponseRecorder.Code)
	}
	// 换 IP 也不能绕过用户桶。
	if res := e.do("GET", "/api/v1/order/user/me", bearer(alice), remote("198.51.100.99:1")); res.ResponseRecorder.Code != 429 {
		t.Errorf("alice from another IP: status %d, want 429", res.ResponseRecorder.Code)
	}
	e.clock.Advance(2 * time.Second)
	if res := e.do("GET", "/api/v1/order/user/me", bearer(alice)); res.ResponseRecorder.Code != 200 {
		t.Errorf("alice after refill: status %d", res.ResponseRecorder.Code)
	}
}

func TestRateLimit_PublicPathsAreNotChargedToAUser(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_USER_RPS": "1", "CK_GATEWAY_USER_BURST": "1"})
	token, _ := e.login(1, "user")
	// 携带令牌访问免登录路径：不校验、不带身份，也不消耗用户桶。
	for i := 0; i < 5; i++ {
		if res := e.do("GET", "/api/v1/coupon/list", bearer(token)); res.ResponseRecorder.Code != 200 {
			t.Fatalf("public request %d: status %d", i, res.ResponseRecorder.Code)
		}
	}
	if res := e.do("GET", "/api/v1/order/user/me", bearer(token)); res.ResponseRecorder.Code != 200 {
		t.Errorf("the user's own budget was spent by public requests: status %d", res.ResponseRecorder.Code)
	}
}

func TestRateLimit_SeckillEntryIsStricterThanOtherPaths(t *testing.T) {
	e := newEnv(t, map[string]string{"CK_GATEWAY_SECKILL_RPS": "1", "CK_GATEWAY_SECKILL_BURST": "2"})
	alice, _ := e.login(1, "user")
	bob, _ := e.login(2, "user")

	for i := 0; i < 2; i++ {
		if res := e.do("POST", "/api/v1/order/seckill", bearer(alice)); res.ResponseRecorder.Code != 200 {
			t.Fatalf("seckill %d: status %d", i, res.ResponseRecorder.Code)
		}
	}
	res := e.do("POST", "/api/v1/order/seckill", bearer(alice))
	if res.ResponseRecorder.Code != 429 || res.Code != apperr.CodeRateLimited {
		t.Fatalf("seckill 3rd: status/code %d/%d, want 429/%d", res.ResponseRecorder.Code, res.Code, apperr.CodeRateLimited)
	}
	if n := e.order.Count(); n != 2 {
		t.Errorf("order upstream saw %d seckill requests, want 2", n)
	}
	// 轮询结果、普通下单都不受秒杀桶限制。
	for _, tc := range [][2]string{
		{"GET", "/api/v1/order/seckill/result?requestId=abc"},
		{"POST", "/api/v1/order/create"},
		{"GET", "/api/v1/order/user/me"},
	} {
		if res := e.do(tc[0], tc[1], bearer(alice)); res.ResponseRecorder.Code != 200 {
			t.Errorf("%s %s: status %d, want 200 (not covered by the seckill bucket)", tc[0], tc[1], res.ResponseRecorder.Code)
		}
	}
	// 秒杀桶按用户计：别的用户不受影响。
	if res := e.do("POST", "/api/v1/order/seckill", bearer(bob)); res.ResponseRecorder.Code != 200 {
		t.Errorf("bob seckill: status %d", res.ResponseRecorder.Code)
	}
	e.clock.Advance(1100 * time.Millisecond)
	if res := e.do("POST", "/api/v1/order/seckill", bearer(alice)); res.ResponseRecorder.Code != 200 {
		t.Errorf("alice seckill after refill: status %d", res.ResponseRecorder.Code)
	}
}

func TestRateLimit_ClientIPHonoursTrustedProxiesOnly(t *testing.T) {
	t.Run("untrusted peer cannot dodge the limit with X-Forwarded-For", func(t *testing.T) {
		e := newEnv(t, map[string]string{"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "2"})
		for i := 0; i < 2; i++ {
			e.do("GET", "/api/v1/coupon/list", hdr("X-Forwarded-For", "10.0.0."+strconv.Itoa(i)))
		}
		res := e.do("GET", "/api/v1/coupon/list", hdr("X-Forwarded-For", "10.9.9.9"))
		if res.ResponseRecorder.Code != 429 {
			t.Fatalf("status = %d, want 429: spoofed X-Forwarded-For must not create new buckets", res.ResponseRecorder.Code)
		}
	})
	t.Run("trusted proxy: the forwarded client address is the bucket key and is passed upstream", func(t *testing.T) {
		e := newEnv(t, map[string]string{
			"CK_GATEWAY_IP_RPS": "1", "CK_GATEWAY_IP_BURST": "1", "CK_GATEWAY_TRUSTED_PROXIES": "203.0.113.0/24",
		})
		viaProxy := func(client string) *result {
			return e.do("GET", "/api/v1/coupon/list", remote("203.0.113.50:1"), hdr("X-Forwarded-For", client), hdr("X-Forwarded-Proto", "https"))
		}
		if res := viaProxy("192.0.2.10"); res.ResponseRecorder.Code != 200 {
			t.Fatalf("first client: status %d", res.ResponseRecorder.Code)
		}
		got := e.coupon.Last(t)
		if got.Header.Get("X-Forwarded-For") != "192.0.2.10" || got.Header.Get("X-Forwarded-Proto") != "https" {
			t.Errorf("upstream saw XFF=%q proto=%q, want the resolved client and https", got.Header.Get("X-Forwarded-For"), got.Header.Get("X-Forwarded-Proto"))
		}
		if res := viaProxy("192.0.2.10"); res.ResponseRecorder.Code != 429 {
			t.Errorf("same client again: status %d, want 429", res.ResponseRecorder.Code)
		}
		if res := viaProxy("192.0.2.11"); res.ResponseRecorder.Code != 200 {
			t.Errorf("a different client behind the same proxy: status %d, want 200", res.ResponseRecorder.Code)
		}
	})
}
