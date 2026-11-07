package reservation

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// RoutePrefix 是本包路由的挂载前缀。
const RoutePrefix = "/api/v1/order/reservations"

// 列表分页：默认与上限都是 100（沿用 Java 的 LIMIT 100，前端“我的预约/按券的活跃预约”一次取全）。
const defaultPageSize = 100

// RegisterRoutes 在 r 上挂载预约接口（完整路径见 RoutePrefix）：
//
//	POST   ""      创建预约 {"couponId":"1001"}（couponId 接受字符串或数字）
//	DELETE /:id    取消预约（仅属主、仅 PENDING），data=true
//	GET    /mine   我的预约（分页，最新在前）
//	GET    /:id    预约详情（别人的/不存在的 404）
//
// r 必须已挂 httpx.RequireGatewayIdentity；即使漏挂，处理器也会因取不到身份而返回 401（失败即关闭）。
func RegisterRoutes(r gin.IRouter, svc *Service) {
	h := &handler{svc: svc}
	g := r.Group(RoutePrefix)
	g.POST("", h.create)
	g.DELETE("/:id", h.cancel)
	g.GET("/mine", h.mine)
	g.GET("/:id", h.get)
}

type handler struct{ svc *Service }

// caller 取调用者的用户 ID；没有有效身份时写出 401 并返回 false。
func caller(c *gin.Context) (int64, bool) {
	id, ok := httpx.IdentityFrom(c)
	if !ok || id.UserID <= 0 {
		httpx.Fail(c, apperr.AuthFail.WithCause(errors.New("缺少网关身份")))
		return 0, false
	}
	return id.UserID, true
}

// createReq 是 POST 的请求体。
type createReq struct {
	CouponID idgen.ID `json:"couponId" binding:"required,gt=0"`
}

func (h *handler) create(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	req, ok := httpx.BindJSON[createReq](c)
	if !ok {
		return
	}
	r, err := h.svc.Create(c.Request.Context(), userID, int64(req.CouponID))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, r)
}

// idURI 是 /:id 的路径参数。
type idURI struct {
	ID int64 `uri:"id" binding:"required,gt=0"`
}

func (h *handler) cancel(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	uri, ok := httpx.BindURI[idURI](c)
	if !ok {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), userID, uri.ID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, true)
}

// listQuery 是 GET /mine 的查询参数；用指针区分“没传”和“传了 0”。
type listQuery struct {
	Page *int `form:"page" binding:"omitempty,min=1"`
	Size *int `form:"size" binding:"omitempty,min=1,max=100"`
}

func (h *handler) mine(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	q, ok := httpx.BindQuery[listQuery](c)
	if !ok {
		return
	}
	page, size := 1, defaultPageSize
	if q.Page != nil {
		page = *q.Page
	}
	if q.Size != nil {
		size = *q.Size
	}
	items, total, err := h.svc.Mine(c.Request.Context(), userID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, total, page, size)
}

func (h *handler) get(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	uri, ok := httpx.BindURI[idURI](c)
	if !ok {
		return
	}
	r, err := h.svc.Get(c.Request.Context(), userID, uri.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, r)
}
