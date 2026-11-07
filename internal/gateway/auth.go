package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// bearerToken 从 Authorization 头取出 Bearer 令牌（scheme 不区分大小写）。令牌只从该头读取，不接受查询参数。
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// authenticate 对需要登录的路径校验令牌：JWT（算法固定、必须带 exp、校验 iss/aud）+ 会话是否仍有效，
// 通过后把已签名的身份头写进转发给上游的请求。免登录路径直接放行，且不携带任何身份。
// 入站的身份头在 Handler 入口已被剥离，所以这里写入的是唯一的身份来源。
func (g *Gateway) authenticate(c *gin.Context) {
	st := stateOf(c)
	if st.public {
		c.Next()
		return
	}
	raw, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		g.fail(c, reasonUnauthenticated, apperr.AuthFail)
		return
	}
	sess, err := g.tokens.Verify(raw)
	if err != nil {
		g.fail(c, reasonTokenInvalid, err)
		return
	}
	if err := g.checkSession(c, sess.JTI); err != nil {
		reason := reasonSessionRevoked
		if errors.Is(err, apperr.SystemBusy) {
			reason = reasonSessionStore
		}
		g.fail(c, reason, err)
		return
	}
	if err := g.applyIdentity(c.Request.Header, sess.Identity); err != nil {
		// 令牌里的角色无法被安全编码进签名头：当作无效令牌处理。
		g.fail(c, reasonTokenInvalid, apperr.TokenInvalid.WithCause(err))
		return
	}
	id := sess.Identity
	st.identity = &id
	ctx := authx.WithIdentity(c.Request.Context(), id)
	c.Request = c.Request.WithContext(logx.With(ctx, "user_id", id.UserID))
	c.Next()
}

// checkSession 查询会话是否有效（吊销检查关闭时直接通过）。
// 会话存储出错或超时（Redis 超时 CK_GATEWAY_REDIS_TIMEOUT）时：fail-open 放行并计数告警，
// fail-closed 返回 503；会话不存在（已登出/已吊销/从未登录）一律 401。
func (g *Gateway) checkSession(c *gin.Context, jti string) error {
	if !g.cfg.RevocationEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), g.cfg.RedisTimeout)
	defer cancel()
	active, err := g.revoker.Active(ctx, jti)
	if err != nil {
		if g.cfg.RevocationFailOpen {
			g.metrics.failOpen.Inc()
			logx.From(c.Request.Context()).Warn("session store unavailable, failing open", "err", err)
			return nil
		}
		return apperr.SystemBusy.WithCause(fmt.Errorf("session store unavailable: %w", err))
	}
	if !active {
		return apperr.AuthFail.WithMessage("登录已失效，请重新登录")
	}
	return nil
}
