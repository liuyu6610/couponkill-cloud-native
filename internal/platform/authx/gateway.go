package authx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 网关身份头名称（design §5）。
const (
	HeaderUserID        = "X-User-Id"
	HeaderUserRoles     = "X-User-Roles"
	HeaderAuthenticated = "X-Authenticated"
	HeaderGatewayTS     = "X-Gateway-Ts"
	HeaderGatewaySig    = "X-Gateway-Sig"

	gatewayHeaderPrefix = "X-Gateway-"
	// GatewayWindow 是网关签名时间戳允许的偏差（双向），用于限制重放窗口。
	GatewayWindow = 60 * time.Second
)

// 验签失败的具体原因；Verify 返回的错误同时满足 errors.Is(err, apperr.AuthFail) 与对应的哨兵。
var (
	ErrMissingIdentity = errors.New("authx: gateway identity headers missing")
	ErrBadSignature    = errors.New("authx: gateway signature mismatch")
	ErrStaleTimestamp  = errors.New("authx: gateway timestamp outside allowed window")
)

// SignGateway 计算 HMAC-SHA256(key, "{userId}|{roles}|{ts}") 的十六进制串，
// roles 以逗号连接，ts 为 Unix 秒。
func SignGateway(key []byte, userID int64, roles []string, ts int64) string {
	return signRaw(key, strconv.FormatInt(userID, 10), strings.Join(roles, ","), strconv.FormatInt(ts, 10))
}

func signRaw(key []byte, user, roles, ts string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(user + "|" + roles + "|" + ts)) // hash.Hash.Write 永不返回错误
	return hex.EncodeToString(m.Sum(nil))
}

// GatewaySigner 负责网关侧签名（Apply）与下游验签（Verify）。
type GatewaySigner struct {
	key []byte
	now func() time.Time
}

// SignerOption 调整 GatewaySigner。
type SignerOption func(*GatewaySigner)

// WithSignerClock 注入时钟（测试用）。
func WithSignerClock(now func() time.Time) SignerOption {
	return func(g *GatewaySigner) { g.now = now }
}

// NewGatewaySigner 构造签名器；密钥不足 32 字节会被拒绝。
func NewGatewaySigner(key string, opts ...SignerOption) (*GatewaySigner, error) {
	if len(key) < minSecretLen {
		return nil, fmt.Errorf("authx: gateway sign key must be at least %d bytes", minSecretLen)
	}
	g := &GatewaySigner{key: []byte(key), now: time.Now}
	for _, o := range opts {
		o(g)
	}
	return g, nil
}

// StripIdentityHeaders 删除所有可能由客户端伪造的身份头：
// X-User-Id、X-User-Roles、X-Authenticated 以及任何 X-Gateway-*。
func StripIdentityHeaders(h http.Header) {
	h.Del(HeaderUserID)
	h.Del(HeaderUserRoles)
	h.Del(HeaderAuthenticated)
	for k := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(k), gatewayHeaderPrefix) {
			delete(h, k)
		}
	}
}

// Apply 先清除已有的身份头，再写入 id 对应的已签名身份头。
func (g *GatewaySigner) Apply(h http.Header, id Identity) error {
	if err := id.validate(); err != nil {
		return err
	}
	StripIdentityHeaders(h)
	ts := g.now().Unix()
	h.Set(HeaderUserID, strconv.FormatInt(id.UserID, 10))
	h.Set(HeaderUserRoles, strings.Join(id.Roles, ","))
	h.Set(HeaderGatewayTS, strconv.FormatInt(ts, 10))
	h.Set(HeaderGatewaySig, SignGateway(g.key, id.UserID, id.Roles, ts))
	return nil
}

// Verify 校验请求头里的网关身份：签名（常量时间比较）与 |now-ts|≤60s。
// 任何失败都返回 apperr.AuthFail，并携带具体原因（ErrMissingIdentity/ErrBadSignature/ErrStaleTimestamp）。
func (g *GatewaySigner) Verify(h http.Header) (Identity, error) {
	user, roles, tsRaw, sigRaw := h.Get(HeaderUserID), h.Get(HeaderUserRoles), h.Get(HeaderGatewayTS), h.Get(HeaderGatewaySig)
	if user == "" || tsRaw == "" || sigRaw == "" {
		return Identity{}, apperr.AuthFail.WithCause(ErrMissingIdentity)
	}
	got, err := hex.DecodeString(sigRaw)
	if err != nil {
		return Identity{}, apperr.AuthFail.WithCause(ErrBadSignature)
	}
	want, _ := hex.DecodeString(signRaw(g.key, user, roles, tsRaw))
	if !hmac.Equal(got, want) { // 先验签：时间戳与用户 ID 在验签通过前不可信
		return Identity{}, apperr.AuthFail.WithCause(ErrBadSignature)
	}
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return Identity{}, apperr.AuthFail.WithCause(ErrBadSignature)
	}
	if d := g.now().Sub(time.Unix(ts, 0)); d > GatewayWindow || d < -GatewayWindow {
		return Identity{}, apperr.AuthFail.WithCause(ErrStaleTimestamp)
	}
	uid, err := strconv.ParseInt(user, 10, 64)
	if err != nil || uid <= 0 || strconv.FormatInt(uid, 10) != user {
		return Identity{}, apperr.AuthFail.WithCause(ErrBadSignature)
	}
	var rs []string
	for _, r := range strings.Split(roles, ",") {
		if r != "" {
			rs = append(rs, r)
		}
	}
	return Identity{UserID: uid, Roles: rs}, nil
}
