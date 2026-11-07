package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Logout 吊销调用方当前持有的令牌（只删这一个 jti 对应的会话，同一用户的其他登录不受影响）。
//
// 网关签名的身份头里没有 jti，所以由调用方把 Authorization 里的原始令牌传进来，这里重新验签取得 jti；
// 令牌的 sub 必须等于网关确认的 id.UserID，否则视为令牌无效，防止拿别人的令牌吊销别人的会话。
// 幂等：会话已不存在不是错误。
func (s *Service) Logout(ctx context.Context, id authx.Identity, rawToken string) error {
	if rawToken == "" {
		return apperr.TokenInvalid.WithMessage("缺少令牌")
	}
	sess, err := s.tokens.Verify(rawToken)
	if err != nil {
		return err
	}
	if sess.Identity.UserID != id.UserID {
		logx.From(ctx).Warn("登出被拒：令牌与网关身份不一致", "token_user_id", sess.Identity.UserID)
		return apperr.TokenInvalid.WithCause(errors.New("token subject does not match gateway identity"))
	}
	if err := s.sessions.Revoke(ctx, sess.JTI); err != nil {
		return apperr.SystemBusy.WithCause(fmt.Errorf("revoke session: %w", err))
	}
	logx.From(ctx).Info("用户登出", "jti", sess.JTI)
	return nil
}

// GetUser 按 ID 取用户视图（不含口令哈希）；不存在返回 apperr.UserNotFound。
// 同时服务于 GET /profile 与内部端点 GET /internal/v1/users/:id。
func (s *Service) GetUser(ctx context.Context, userID int64) (User, error) {
	u, err := s.repo.FindByID(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return User{}, apperr.UserNotFound.WithCause(err)
	case err != nil:
		return User{}, fmt.Errorf("find user %d: %w", userID, err)
	}
	return u, nil
}
