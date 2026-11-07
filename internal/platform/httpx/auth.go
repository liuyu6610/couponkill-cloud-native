package httpx

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

const ctxKeyIdentity = "httpx.identity"

// IdentityFrom 取出 RequireGatewayIdentity 放进 gin.Context 的身份。
func IdentityFrom(c *gin.Context) (authx.Identity, bool) {
	v, ok := c.Get(ctxKeyIdentity)
	if !ok {
		return authx.Identity{}, false
	}
	id, ok := v.(authx.Identity)
	return id, ok
}

// RequireGatewayIdentity 校验网关签名的身份头（签名 + ±60s 时间窗），通过后把身份放进
// gin.Context（IdentityFrom）与请求 ctx（authx.IdentityFrom），并让后续日志带上 user_id。
// 失败一律 401：下游因此不信任任何未经网关签名的身份头，绕过网关直连也无法伪造身份。
func RequireGatewayIdentity(signer *authx.GatewaySigner) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := signer.Verify(c.Request.Header)
		if err != nil {
			Fail(c, err)
			return
		}
		c.Set(ctxKeyIdentity, id)
		ctx := authx.WithIdentity(c.Request.Context(), id)
		ctx = logx.With(ctx, "user_id", id.UserID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequireInternalToken 要求 X-Internal-Token 与 token 相等（常量时间比较）。
// token 为空时拒绝所有请求（失败即关闭），避免漏配密钥时内部接口裸奔。
func RequireInternalToken(token string) gin.HandlerFunc {
	want := sha256.Sum256([]byte(token))
	return func(c *gin.Context) {
		got := sha256.Sum256([]byte(c.GetHeader(HeaderInternalToken)))
		if token == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			Fail(c, apperr.AuthFail.WithMessage("内部调用凭证无效").WithCause(errors.New("bad internal token")))
			return
		}
		c.Next()
	}
}

// RequireRole 要求已认证身份至少带有 roles 中的一个。未认证 401，角色不足 403。
// 必须放在 RequireGatewayIdentity 之后。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := IdentityFrom(c)
		if !ok {
			Fail(c, apperr.AuthFail.WithCause(errors.New("RequireRole used without an authenticated identity")))
			return
		}
		for _, r := range roles {
			if id.HasRole(r) {
				c.Next()
				return
			}
		}
		Fail(c, apperr.Forbidden.WithCause(errors.New("missing required role")))
	}
}

// RateLimit 按 keyFn 给出的 key 做令牌桶限流，超限返回 429 + Retry-After。
// keyFn 返回空串表示不限流（例如拿不到用户身份时只靠 IP 桶）。
func RateLimit(l *limiter.Buckets, keyFn func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			c.Next()
			return
		}
		if d := l.Take(key); !d.Allowed {
			if d.RetryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(int(math.Ceil(d.RetryAfter.Seconds()))))
			}
			Fail(c, apperr.RateLimited)
			return
		}
		c.Next()
	}
}
