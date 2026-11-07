// Package connector 是电商 Connector 服务（端口 8085）：平台 SKU 绑定、同品比价、平台探测，
// 以及把平台库存定时同步给 coupon 服务。本文件是 HTTP 路由层（薄适配），业务在 service 子包，
// 平台适配在 platforms 子包，存储在 repo 子包。
package connector

import (
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
)

// BasePath 是 connector 公开 API 的路径前缀。
const BasePath = "/api/v1/connector"

// JDInfo 是 /platforms 展示京东配置就绪态所需的只读信息（不含任何密钥）。
type JDInfo interface {
	CredentialsConfigured() bool
	ServerURL() string
	DefaultArea() string
	AppKeyMasked() string
}

// Deps 是路由层依赖的服务。
type Deps struct {
	Bindings *service.BindingService
	Prices   *service.PriceService
	Probe    *service.ProbeService
	Sync     *service.SyncService
	// Signer 验证网关签名的身份头。
	Signer *authx.GatewaySigner
	// JD 可选；提供时 /platforms 的 JD 条目带上京东配置就绪态。
	JD JDInfo
	// Logger 是访问日志用的 logger，默认 slog.Default()。
	Logger *slog.Logger
}

// NewEngine 构造 connector 的 HTTP 引擎（含 /healthz /readyz /metrics，由 httpx 提供）。
//
// 鉴权矩阵：
//   - GET /health：无需身份（网关免登录白名单）。
//   - GET /bindings/by-coupon/{id}、GET /price-compare：C 端只读，网关对匿名请求放行且不带身份头，
//     因此这里允许“没有任何身份头”的请求；一旦带了身份头就必须签名有效（伪造 → 401）。
//   - 其余全部：必须有网关签名的身份，并且角色含 admin（普通用户 403，匿名 401）。
func NewEngine(d Deps) *httpx.Engine {
	eng := httpx.NewEngine(httpx.Options{Service: "connector", Logger: d.Logger})
	h := &handlers{d: d}

	v1 := eng.Group(BasePath)
	v1.GET("/health", h.health)

	public := v1.Group("", optionalIdentity(d.Signer))
	public.GET("/bindings/by-coupon/:couponId", h.bindingByCoupon)
	public.GET("/price-compare", h.priceCompare)

	admin := v1.Group("", httpx.RequireGatewayIdentity(d.Signer), httpx.RequireRole("admin"))
	admin.POST("/bindings", h.upsertBinding)
	admin.GET("/bindings", h.listBindings)
	admin.POST("/price-maps", h.upsertPriceMap)
	admin.GET("/price-maps", h.listPriceMaps)
	admin.DELETE("/price-maps/:id", h.deletePriceMap)
	admin.POST("/sync/:id", h.syncOne)
	admin.POST("/sync", h.syncAll)
	admin.GET("/platforms", h.platforms)
	admin.GET("/probe/:platform/product", h.probeProduct)
	admin.GET("/probe/:platform/stock", h.probeStock)
	return eng
}

// optionalIdentity 允许完全匿名的请求通过；只要带了任何网关身份头，就必须通过签名校验。
func optionalIdentity(signer *authx.GatewaySigner) gin.HandlerFunc {
	required := httpx.RequireGatewayIdentity(signer)
	return func(c *gin.Context) {
		hdr := c.Request.Header
		if hdr.Get(authx.HeaderGatewaySig) == "" && hdr.Get(authx.HeaderUserID) == "" &&
			hdr.Get(authx.HeaderUserRoles) == "" && hdr.Get(authx.HeaderGatewayTS) == "" {
			c.Next()
			return
		}
		required(c)
	}
}

type handlers struct{ d Deps }

// parseID 解析路径或查询里的正整数 ID；非法返回 400。
func parseID(name, raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.InvalidRequest.WithMessage(name + " 必须是正整数")
	}
	return id, nil
}

type couponQuery struct {
	CouponID string `form:"couponId" binding:"required"`
}

type forceQuery struct {
	Force bool `form:"force"`
}

type skuQuery struct {
	SkuID string `form:"skuId" binding:"required"`
}

func (h *handlers) health(c *gin.Context) {
	httpx.OK(c, h.d.Probe.Health(c.Request.Context()))
}

func (h *handlers) bindingByCoupon(c *gin.Context) {
	id, err := parseID("couponId", c.Param("couponId"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	b, err := h.d.Bindings.ByCoupon(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, b) // 没有绑定时 data 为 null（前端按“未绑定”处理）
}

func (h *handlers) priceCompare(c *gin.Context) {
	q, ok := httpx.BindQuery[couponQuery](c)
	if !ok {
		return
	}
	id, err := parseID("couponId", q.CouponID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	res, err := h.d.Prices.Compare(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *handlers) upsertBinding(c *gin.Context) {
	cmd, ok := httpx.BindJSON[domain.BindingCommand](c)
	if !ok {
		return
	}
	b, err := h.d.Bindings.Upsert(c.Request.Context(), cmd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, b)
}

func (h *handlers) listBindings(c *gin.Context) {
	list, err := h.d.Bindings.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handlers) upsertPriceMap(c *gin.Context) {
	cmd, ok := httpx.BindJSON[domain.PriceMapCommand](c)
	if !ok {
		return
	}
	pm, err := h.d.Prices.Upsert(c.Request.Context(), cmd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pm)
}

func (h *handlers) listPriceMaps(c *gin.Context) {
	q, ok := httpx.BindQuery[couponQuery](c)
	if !ok {
		return
	}
	id, err := parseID("couponId", q.CouponID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	list, err := h.d.Prices.List(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, list)
}

func (h *handlers) deletePriceMap(c *gin.Context) {
	id, err := parseID("id", c.Param("id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.d.Prices.Delete(c.Request.Context(), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handlers) syncOne(c *gin.Context) {
	id, err := parseID("id", c.Param("id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	q, ok := httpx.BindQuery[forceQuery](c)
	if !ok {
		return
	}
	b, err := h.d.Sync.SyncOne(c.Request.Context(), id, q.Force)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, b)
}

func (h *handlers) syncAll(c *gin.Context) {
	q, ok := httpx.BindQuery[forceQuery](c)
	if !ok {
		return
	}
	res, err := h.d.Sync.SyncAll(c.Request.Context(), q.Force)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// platformInfo 是 /platforms 的一项：健康状态，京东条目额外带配置就绪态（不含密钥）。
type platformInfo struct {
	Platform                domain.Platform `json:"platform"`
	Status                  string          `json:"status"`
	Message                 string          `json:"message"`
	JDEnabled               *bool           `json:"jdEnabled,omitempty"`
	JDCredentialsConfigured *bool           `json:"jdCredentialsConfigured,omitempty"`
	JDServerURL             *string         `json:"jdServerUrl,omitempty"`
	JDDefaultArea           *string         `json:"jdDefaultArea,omitempty"`
	JDAppKeyMasked          *string         `json:"jdAppKeyMasked,omitempty"`
}

func (h *handlers) platforms(c *gin.Context) {
	healths := h.d.Probe.Health(c.Request.Context())
	out := make([]platformInfo, 0, len(healths))
	for _, hl := range healths {
		info := platformInfo{Platform: hl.Platform, Status: hl.Status, Message: hl.Message}
		if hl.Platform == domain.JD && h.d.JD != nil {
			configured, url, area, masked := h.d.JD.CredentialsConfigured(), h.d.JD.ServerURL(), h.d.JD.DefaultArea(), h.d.JD.AppKeyMasked()
			// 没有单独的启用开关：凭证齐全即启用。
			info.JDEnabled, info.JDCredentialsConfigured = &configured, &configured
			info.JDServerURL, info.JDDefaultArea, info.JDAppKeyMasked = &url, &area, &masked
		}
		out = append(out, info)
	}
	httpx.OK(c, out)
}

func (h *handlers) probeProduct(c *gin.Context) {
	q, ok := httpx.BindQuery[skuQuery](c)
	if !ok {
		return
	}
	snap, err := h.d.Probe.Product(c.Request.Context(), c.Param("platform"), q.SkuID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, snap)
}

func (h *handlers) probeStock(c *gin.Context) {
	q, ok := httpx.BindQuery[skuQuery](c)
	if !ok {
		return
	}
	snap, err := h.d.Probe.Stock(c.Request.Context(), c.Param("platform"), q.SkuID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, snap)
}
