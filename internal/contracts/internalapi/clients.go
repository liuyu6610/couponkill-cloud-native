package internalapi

import (
	"context"
	"net/http"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 使用方（order/coupon/connector 的 service 层）应只依赖下面这些小接口，
// 测试时用假实现替换；类型化客户端在编译期保证满足它们。
type (
	// StockDeductor 扣减库存（按 requestId 幂等）。
	StockDeductor interface {
		DeductStock(ctx context.Context, req DeductRequest) (DeductResponse, error)
	}
	// StockReleaser 归还库存（按 requestId 幂等）。
	StockReleaser interface {
		ReleaseStock(ctx context.Context, req ReleaseRequest) (ReleaseResponse, error)
	}
	// StockSyncer 把券的可用库存设置为绝对值。
	StockSyncer interface {
		SyncStock(ctx context.Context, req SyncRequest) (SyncResponse, error)
	}
	// CouponGetter 读取券详情。
	CouponGetter interface {
		GetCoupon(ctx context.Context, couponID int64) (CouponInfo, error)
	}
	// OrderGranter 给用户直接建单（按 requestId 幂等）。
	OrderGranter interface {
		GrantOrder(ctx context.Context, req GrantRequest) (GrantResponse, error)
	}
	// UserGetter 读取用户基本信息。
	UserGetter interface {
		GetUser(ctx context.Context, userID int64) (UserInfo, error)
	}
)

var (
	_ StockDeductor = (*CouponClient)(nil)
	_ StockReleaser = (*CouponClient)(nil)
	_ StockSyncer   = (*CouponClient)(nil)
	_ CouponGetter  = (*CouponClient)(nil)
	_ OrderGranter  = (*OrderClient)(nil)
	_ UserGetter    = (*UserClient)(nil)
)

// CouponClient 是 coupon 服务内部 API 的客户端，并发安全。
//
// 重试：DeductStock、ReleaseStock、GetCoupon 按 Option 里的策略重试 transient 失败；
// SyncStock 不重试（连接器自己有重试循环，避免用旧值覆盖更新的同步）。
type CouponClient struct{ t *transport }

// NewCouponClient 创建 coupon 客户端。baseURL 如 "http://couponkill-coupon:8080"；
// token 是 CK_INTERNAL_TOKEN；hc 为 nil 时使用 DefaultHTTPClient()。
func NewCouponClient(baseURL, token string, hc *http.Client, opts ...Option) (*CouponClient, error) {
	t, err := newTransport("coupon", baseURL, token, hc, apperr.CouponServiceUnavailable, opts)
	if err != nil {
		return nil, err
	}
	return &CouponClient{t: t}, nil
}

// DeductStock 调用 POST /internal/v1/stock/deduct。幂等，transient 失败自动重试。
// 售罄返回 409/OutOfStock（不重试）；耗尽重试返回满足 IsTransient 的错误。
func (c *CouponClient) DeductStock(ctx context.Context, req DeductRequest) (DeductResponse, error) {
	var out DeductResponse
	if err := req.Validate(); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "deduct stock", http.MethodPost, PathStockDeduct, req, &out, true)
	return out, err
}

// ReleaseStock 调用 POST /internal/v1/stock/release。幂等，transient 失败自动重试。
func (c *CouponClient) ReleaseStock(ctx context.Context, req ReleaseRequest) (ReleaseResponse, error) {
	var out ReleaseResponse
	if err := req.Validate(); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "release stock", http.MethodPost, PathStockRelease, req, &out, true)
	return out, err
}

// SyncStock 调用 POST /internal/v1/stock/sync。不自动重试。
func (c *CouponClient) SyncStock(ctx context.Context, req SyncRequest) (SyncResponse, error) {
	var out SyncResponse
	if err := req.Validate(); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "sync stock", http.MethodPost, PathStockSync, req, &out, false)
	return out, err
}

// GetCoupon 调用 GET /internal/v1/coupons/{id}。只读，transient 失败自动重试。
// 券不存在返回 404/CouponNotFound。
func (c *CouponClient) GetCoupon(ctx context.Context, couponID int64) (CouponInfo, error) {
	var out CouponInfo
	if err := checkPositive("couponId", couponID); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "get coupon", http.MethodGet, CouponPath(couponID), nil, &out, true)
	return out, err
}

// OrderClient 是 order 服务内部 API 的客户端，并发安全。
type OrderClient struct{ t *transport }

// NewOrderClient 创建 order 客户端，参数含义同 NewCouponClient。
func NewOrderClient(baseURL, token string, hc *http.Client, opts ...Option) (*OrderClient, error) {
	t, err := newTransport("order", baseURL, token, hc, apperr.SystemBusy, opts)
	if err != nil {
		return nil, err
	}
	return &OrderClient{t: t}, nil
}

// GrantOrder 调用 POST /internal/v1/orders/grant。按 requestId 幂等，transient 失败自动重试。
// 用户已持有该券返回 409/RepeatSeckill（不重试）。
func (c *OrderClient) GrantOrder(ctx context.Context, req GrantRequest) (GrantResponse, error) {
	var out GrantResponse
	if err := req.Validate(); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "grant order", http.MethodPost, PathOrderGrant, req, &out, true)
	return out, err
}

// UserClient 是 user 服务内部 API 的客户端，并发安全。
type UserClient struct{ t *transport }

// NewUserClient 创建 user 客户端，参数含义同 NewCouponClient。
func NewUserClient(baseURL, token string, hc *http.Client, opts ...Option) (*UserClient, error) {
	t, err := newTransport("user", baseURL, token, hc, apperr.SystemBusy, opts)
	if err != nil {
		return nil, err
	}
	return &UserClient{t: t}, nil
}

// GetUser 调用 GET /internal/v1/users/{id}。只读，transient 失败自动重试。
// 用户不存在返回 404/UserNotFound。
func (c *UserClient) GetUser(ctx context.Context, userID int64) (UserInfo, error) {
	var out UserInfo
	if err := checkPositive("userId", userID); err != nil {
		return out, err
	}
	err := c.t.call(ctx, "get user", http.MethodGet, UserPath(userID), nil, &out, true)
	return out, err
}
