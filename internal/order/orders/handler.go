package orders

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// Handler 是订单域的 HTTP 适配层。
type Handler struct {
	svc *Service
}

// NewHandler 创建处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 在 r（/api/v1/order 分组，必须已挂 RequireGatewayIdentity）上注册用户与管理端路由：
//
//	POST /create            {couponId}（也接受查询参数）→ 订单
//	POST /cancel            {orderId}（也接受查询参数）→ true/false
//	GET  /check/:couponId   → true/false（是否持有未取消订单）
//	GET  /user/me           ?page&size（兼容 pageNum/pageSize）→ 分页订单
//	GET  /user/:userId      管理员；同上
//	GET  /admin             管理员；?page&size&startTime&endTime&userId&couponId&status&orderId
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/create", h.create)
	r.POST("/cancel", h.cancel)
	r.GET("/check/:couponId", h.check)
	r.GET("/user/me", h.myOrders)
	r.GET("/user/:userId", httpx.RequireRole(authx.RoleAdmin), h.userOrders)
	r.GET("/admin", httpx.RequireRole(authx.RoleAdmin), h.adminOrders)
}

// RegisterUser 在 r（/api/v1/user 分组，必须已挂 RequireGatewayIdentity）上注册：
//
//	GET /coupon/count       → {count,userId,totalCount,seckillCount,normalCount,expiredCount}
func (h *Handler) RegisterUser(r gin.IRouter) {
	r.GET("/coupon/count", h.couponCount)
}

// RegisterInternal 在 r（必须已挂 RequireInternalToken）上注册 POST /internal/v1/orders/grant。
func (h *Handler) RegisterInternal(r gin.IRouter) {
	r.POST(internalapi.PathOrderGrant, h.grant)
}

func identity(c *gin.Context) (authx.Identity, bool) {
	id, ok := httpx.IdentityFrom(c)
	if !ok {
		httpx.Fail(c, apperr.AuthFail)
	}
	return id, ok
}

func (h *Handler) create(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	couponID, ok := IDParam(c, "couponId")
	if !ok {
		return
	}
	order, err := h.svc.Create(c.Request.Context(), id.UserID, couponID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, order)
}

func (h *Handler) cancel(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	orderID, ok := IDParam(c, "orderId")
	if !ok {
		return
	}
	done, err := h.svc.Cancel(c.Request.Context(), id.UserID, orderID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, done)
}

func (h *Handler) check(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	uri, ok := httpx.BindURI[struct {
		CouponID idgen.ID `uri:"couponId" binding:"required,gt=0"`
	}](c)
	if !ok {
		return
	}
	held, err := h.svc.Holds(c.Request.Context(), id.UserID, int64(uri.CouponID))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, held)
}

// pageQuery 同时接受新的 page/size 与旧前端的 pageNum/pageSize。
type pageQuery struct {
	Page     int `form:"page"`
	Size     int `form:"size"`
	PageNum  int `form:"pageNum"`
	PageSize int `form:"pageSize"`
}

func (q pageQuery) request() PageRequest {
	p := PageRequest{Page: q.Page, Size: q.Size}
	if p.Page == 0 {
		p.Page = q.PageNum
	}
	if p.Size == 0 {
		p.Size = q.PageSize
	}
	return p
}

func (h *Handler) myOrders(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	h.listUser(c, id.UserID)
}

func (h *Handler) userOrders(c *gin.Context) {
	uri, ok := httpx.BindURI[struct {
		UserID idgen.ID `uri:"userId" binding:"required,gt=0"`
	}](c)
	if !ok {
		return
	}
	h.listUser(c, int64(uri.UserID))
}

func (h *Handler) listUser(c *gin.Context, userID int64) {
	q, ok := httpx.BindQuery[pageQuery](c)
	if !ok {
		return
	}
	page, err := h.svc.ListByUser(c.Request.Context(), userID, q.request())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, page.Items, page.Total, page.Page, page.Size)
}

// adminQuery 是管理端列表的查询参数。
type adminQuery struct {
	pageQuery
	StartTime string   `form:"startTime"`
	EndTime   string   `form:"endTime"`
	UserID    idgen.ID `form:"userId"`
	CouponID  idgen.ID `form:"couponId"`
	OrderID   idgen.ID `form:"orderId"`
	Status    int      `form:"status" binding:"min=0,max=4"`
}

func (h *Handler) adminOrders(c *gin.Context) {
	q, ok := httpx.BindQuery[adminQuery](c)
	if !ok {
		return
	}
	filter := AdminFilter{UserID: int64(q.UserID), CouponID: int64(q.CouponID), OrderID: int64(q.OrderID), Status: q.Status}
	var err error
	if filter.From, err = parseTime(q.StartTime); err != nil {
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: startTime 格式不正确").WithCause(err))
		return
	}
	if filter.To, err = parseTime(q.EndTime); err != nil {
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: endTime 格式不正确").WithCause(err))
		return
	}
	page, err := h.svc.ListAll(c.Request.Context(), filter, q.request())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, page.Items, page.Total, page.Page, page.Size)
}

// timeLayouts 是管理端时间过滤接受的格式（与 Java 版 CAST AS TIMESTAMP 的宽松程度相当）。
var timeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

// parseTime 解析时间过滤参数；空串表示不过滤。不带时区的格式按 UTC 解释。
func parseTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var firstErr error
	for _, layout := range timeLayouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return &t, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// couponCountResponse 同时带 count（= 持有总数，前端契约）与按类型细分的统计。
type couponCountResponse struct {
	Count  int      `json:"count"`
	UserID idgen.ID `json:"userId"`
	Counts
}

func (h *Handler) couponCount(c *gin.Context) {
	id, ok := identity(c)
	if !ok {
		return
	}
	counts, err := h.svc.Counts(c.Request.Context(), id.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, couponCountResponse{Count: counts.Total, UserID: idgen.ID(id.UserID), Counts: counts})
}

func (h *Handler) grant(c *gin.Context) {
	req, ok := httpx.BindJSON[internalapi.GrantRequest](c)
	if !ok {
		return
	}
	resp, err := h.svc.Grant(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, resp)
}
