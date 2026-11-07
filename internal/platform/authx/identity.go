// Package authx 汇集鉴权相关的原语：HS256 JWT、口令哈希、网关身份头签名、会话吊销接口。
//
// 信任模型（design §5）：只有网关校验 JWT；网关通过 HMAC 签名的身份头把身份传给下游，
// 下游只信带有效签名且时间戳在 ±60s 内的身份头，因此绕过网关直连也无法伪造身份。
package authx

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// 角色名常量。
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Identity 是已认证调用方的身份。
type Identity struct {
	UserID int64
	Roles  []string
}

// HasRole 报告身份是否带有某个角色。
func (i Identity) HasRole(role string) bool { return slices.Contains(i.Roles, role) }

// IsAdmin 报告身份是否为管理员。
func (i Identity) IsAdmin() bool { return i.HasRole(RoleAdmin) }

// validate 检查身份能否被安全编码进令牌/签名头：用户 ID 为正，角色名非空且不含 , | 与空白。
func (i Identity) validate() error {
	if i.UserID <= 0 {
		return fmt.Errorf("authx: invalid user id %d", i.UserID)
	}
	for _, r := range i.Roles {
		if r == "" || strings.ContainsAny(r, ",| \t\r\n") {
			return fmt.Errorf("authx: invalid role %q", r)
		}
	}
	return nil
}

type ctxKey struct{}

// WithIdentity 把身份放进 ctx，供业务层读取调用方。
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFrom 取出 ctx 里的身份。
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}
