package authx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// minSecretLen 是 HMAC 密钥的最小字节数（HS256 的摘要长度）。
const minSecretLen = 32

// TokenConfig 配置 JWT 的签发与校验。
type TokenConfig struct {
	// Secret HS256 密钥，至少 32 字节。
	Secret string
	// Issuer / Audience 签发时写入，校验时必须匹配。
	Issuer   string
	Audience string
	// TTL 令牌有效期，默认 24h。
	TTL time.Duration
	// Leeway 校验时容忍的时钟偏差，默认 30s。
	Leeway time.Duration
	// Now 注入时钟（测试用），默认 time.Now。
	Now func() time.Time
}

// Token 是一次签发的结果。
type Token struct {
	// Raw 是紧凑序列化的 JWT。
	Raw string
	// JTI 是令牌唯一 ID，也是会话键 ck:sess:{jti} 的后缀。
	JTI       string
	ExpiresAt time.Time
}

// Session 是校验通过的令牌内容。
type Session struct {
	Identity  Identity
	JTI       string
	ExpiresAt time.Time
}

// Tokens 签发并校验 JWT。零值不可用，请用 NewTokens。
type Tokens struct {
	cfg    TokenConfig
	secret []byte
	parser *jwt.Parser
}

type claims struct {
	jwt.RegisteredClaims
	Roles []string `json:"roles"`
}

// NewTokens 校验配置并构造 Tokens：密钥不足 32 字节、iss/aud 为空都会被拒绝。
func NewTokens(cfg TokenConfig) (*Tokens, error) {
	if len(cfg.Secret) < minSecretLen {
		return nil, fmt.Errorf("authx: jwt secret must be at least %d bytes", minSecretLen)
	}
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("authx: jwt issuer and audience are required")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), // 固定算法，杜绝 alg=none 与算法混淆
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithLeeway(cfg.Leeway),
		jwt.WithTimeFunc(cfg.Now),
	)
	return &Tokens{cfg: cfg, secret: []byte(cfg.Secret), parser: parser}, nil
}

// Issue 为身份签发新令牌（每次都有新的 jti）。
func (t *Tokens) Issue(id Identity) (Token, error) {
	if err := id.validate(); err != nil {
		return Token{}, err
	}
	var b [16]byte
	_, _ = rand.Read(b[:]) // Go 1.24+ 永不返回错误
	jti := hex.EncodeToString(b[:])
	now := t.cfg.Now().Truncate(time.Second)
	exp := now.Add(t.cfg.TTL)
	roles := append([]string(nil), id.Roles...)
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(id.UserID, 10),
			Issuer:    t.cfg.Issuer,
			Audience:  jwt.ClaimStrings{t.cfg.Audience},
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Roles: roles,
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return Token{}, fmt.Errorf("authx: sign jwt: %w", err)
	}
	return Token{Raw: raw, JTI: jti, ExpiresAt: exp}, nil
}

// Verify 校验令牌并取出会话。过期返回 apperr.AuthFail，其余一切不合法都返回 apperr.TokenInvalid
// （原因保留为 cause 供日志使用，不会出现在响应里）。
func (t *Tokens) Verify(raw string) (Session, error) {
	var c claims
	_, err := t.parser.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return t.secret, nil })
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Session{}, apperr.AuthFail.WithCause(err)
		}
		return Session{}, apperr.TokenInvalid.WithCause(err)
	}
	uid, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || uid <= 0 {
		return Session{}, apperr.TokenInvalid.WithCause(fmt.Errorf("invalid sub claim %q", c.Subject))
	}
	if c.ID == "" {
		return Session{}, apperr.TokenInvalid.WithCause(errors.New("missing jti claim"))
	}
	return Session{
		Identity:  Identity{UserID: uid, Roles: c.Roles},
		JTI:       c.ID,
		ExpiresAt: c.ExpiresAt.Time,
	}, nil
}
