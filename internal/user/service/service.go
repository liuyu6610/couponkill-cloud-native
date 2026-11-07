package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// LastActiveRefreshInterval 是登录刷新 last_active_at 的最小间隔（设计 §10：每用户每小时至多一次）。
const LastActiveRefreshInterval = time.Hour

// Deps 是 Service 的依赖。Repo、Sessions、Tokens、IDs 必填；Hasher 缺省为 AuthxHasher，
// Now 缺省为 time.Now，Logger 缺省为 slog.Default()。
type Deps struct {
	Repo     Repo
	Sessions Sessions
	Tokens   Tokens
	Hasher   PasswordHasher
	IDs      IDGenerator
	Now      func() time.Time
	Logger   *slog.Logger
}

// Service 承载 user 服务的全部业务规则，并发安全（自身无可变状态）。
type Service struct {
	repo     Repo
	sessions Sessions
	tokens   Tokens
	hasher   PasswordHasher
	ids      IDGenerator
	now      func() time.Time
	log      *slog.Logger
}

// New 校验依赖并构造 Service。
func New(d Deps) (*Service, error) {
	switch {
	case d.Repo == nil:
		return nil, errors.New("user service: Repo is required")
	case d.Sessions == nil:
		return nil, errors.New("user service: Sessions is required")
	case d.Tokens == nil:
		return nil, errors.New("user service: Tokens is required")
	case d.IDs == nil:
		return nil, errors.New("user service: IDs is required")
	}
	s := &Service{
		repo: d.Repo, sessions: d.Sessions, tokens: d.Tokens, hasher: d.Hasher,
		ids: d.IDs, now: d.Now, log: d.Logger,
	}
	if s.hasher == nil {
		s.hasher = AuthxHasher{}
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	return s, nil
}

// RegisterInput 是注册请求。
type RegisterInput struct {
	Username string
	Password string
	Phone    string
}

// Register 校验输入、哈希口令并创建普通用户（角色永远是 "user"，不接受调用方指定）。
// 用户名重复返回 apperr.UserExist：唯一性完全由数据库唯一约束保证，并发同名注册恰有一个成功。
func (s *Service) Register(ctx context.Context, in RegisterInput) (User, error) {
	if err := validateUsername(in.Username); err != nil {
		return User{}, err
	}
	if err := validateNewPassword(in.Password); err != nil {
		return User{}, err
	}
	if err := validatePhone(in.Phone); err != nil {
		return User{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := s.repo.Create(ctx, NewAccount{
		ID: s.ids.Next(), Username: in.Username, PasswordHash: hash, Phone: in.Phone, Role: authx.RoleUser,
	})
	switch {
	case errors.Is(err, ErrUsernameTaken):
		logx.From(ctx).Info("注册失败：用户名已存在", "username", in.Username)
		return User{}, apperr.UserExist.WithCause(err)
	case err != nil:
		return User{}, fmt.Errorf("create user: %w", err)
	}
	logx.From(ctx).Info("用户注册成功", "user_id", u.ID, "username", u.Username)
	return u, nil
}

// LoginInput 是登录请求。
type LoginInput struct {
	Username string
	Password string
}

// LoginResult 是登录成功的结果。
type LoginResult struct {
	// Token 是 JWT 原文。
	Token string
	// User 是登录的用户视图。
	User User
	// Roles 是写入令牌的角色：管理员为 [admin user]，普通用户为 [user]。
	Roles []string
	// ExpiresAt 是令牌过期时间。
	ExpiresAt time.Time
}

// rolesFor 把 users.role 展开为令牌里的角色列表（沿用 Java：管理员同时拥有 user）。
func rolesFor(role string) []string {
	if role == authx.RoleAdmin {
		return []string{authx.RoleAdmin, authx.RoleUser}
	}
	return []string{authx.RoleUser}
}

// Login 校验凭证并签发令牌。
//
// 防枚举与防时序：无论用户是否存在，都恰好调用一次 VerifyOrDummy（用户不存在时 hash 传空串，
// 做等价工作量的哑比较），并且“用户不存在”与“口令错误”返回完全相同的 apperr.PasswordError，
// 文案、错误码、状态码都一致。只有口令校验通过之后才会暴露“账号已停用”。
// 成功后登记会话 ck:sess:{jti}（TTL=令牌剩余寿命），并按 LastActiveRefreshInterval 节流刷新 last_active_at。
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	if err := validateLogin(in); err != nil {
		return LoginResult{}, err
	}
	log := logx.From(ctx)

	acc, err := s.repo.FindByUsername(ctx, in.Username)
	found := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}
	hash := ""
	if found {
		hash = acc.PasswordHash
	}
	passwordOK := s.hasher.VerifyOrDummy(hash, in.Password)
	if !found || !passwordOK {
		log.Info("登录失败", "username", in.Username, "user_found", found)
		return LoginResult{}, apperr.PasswordError
	}
	if acc.Status != StatusActive {
		log.Info("登录被拒：账号已停用", "user_id", acc.ID)
		return LoginResult{}, apperr.Forbidden.WithMessage("账号已停用")
	}

	roles := rolesFor(acc.Role)
	tok, err := s.tokens.Issue(authx.Identity{UserID: acc.ID, Roles: roles})
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}
	ttl := tok.ExpiresAt.Sub(s.now())
	if ttl <= 0 {
		return LoginResult{}, fmt.Errorf("issued token already expired (exp %v)", tok.ExpiresAt)
	}
	if err := s.sessions.Open(ctx, tok.JTI, ttl); err != nil {
		log.Error("登记登录会话失败", "user_id", acc.ID, "error", err)
		return LoginResult{}, apperr.SystemBusy.WithCause(fmt.Errorf("open session: %w", err))
	}
	s.touchLastActive(ctx, acc)

	log.Info("用户登录成功", "user_id", acc.ID, "jti", tok.JTI)
	return LoginResult{Token: tok.Raw, User: acc.User, Roles: roles, ExpiresAt: tok.ExpiresAt}, nil
}

// touchLastActive 尽力刷新 last_active_at：已读到的值仍新鲜时直接跳过，省一次写；
// 真正的频率上限由仓储的条件更新保证。失败只记日志，不影响登录。
func (s *Service) touchLastActive(ctx context.Context, acc Account) {
	now := s.now()
	if acc.LastActiveAt != nil && now.Sub(*acc.LastActiveAt) < LastActiveRefreshInterval {
		return
	}
	if _, err := s.repo.TouchLastActive(ctx, acc.ID, now, LastActiveRefreshInterval); err != nil {
		logx.From(ctx).Warn("刷新 last_active_at 失败", "user_id", acc.ID, "error", err)
	}
}

// validateLogin 在任何查询之前拒绝空凭证；口令超过 bcrypt 上限的一律视为口令错误，
// 因为注册时不可能创建这样的口令，不必查库，也不会泄露任何用户信息。
func validateLogin(in LoginInput) error {
	if strings.TrimSpace(in.Username) == "" || in.Password == "" {
		return invalid("用户名和密码不能为空")
	}
	if len(in.Password) > MaxPasswordBytes {
		return apperr.PasswordError
	}
	return nil
}
