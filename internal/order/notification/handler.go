package notification

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
)

// defaultPageSize 是默认页大小（沿用 Java 的 limit 默认 20）；上限 50 由 listQuery 的 tag 强制。
const defaultPageSize = 20

// RoutePrefix 是本包路由的挂载前缀。
const RoutePrefix = "/api/v1/order/notifications"

// RegisterRoutes 在 r 上挂载通知接口（完整路径见 RoutePrefix）：
//
//	GET  /mine          我的通知（分页，最新在前）
//	GET  /unread-count  未读数 {"count": N}
//	POST /:id/read      单条已读（别人的/不存在的 404，重复调用 200）
//	POST /read-all      全部已读，返回本次新标记的条数
//
// r 必须已挂 httpx.RequireGatewayIdentity；即使漏挂，处理器也会因取不到身份而返回 401（失败即关闭）。
func RegisterRoutes(r gin.IRouter, svc *Service) {
	h := &handler{svc: svc}
	g := r.Group(RoutePrefix)
	g.GET("/mine", h.mine)
	g.GET("/unread-count", h.unreadCount)
	g.POST("/read-all", h.readAll)
	g.POST("/:id/read", h.markRead)
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

// listQuery 是 GET /mine 的查询参数；用指针区分“没传”和“传了 0”。
type listQuery struct {
	Page *int `form:"page" binding:"omitempty,min=1"`
	Size *int `form:"size" binding:"omitempty,min=1,max=50"`
	// Limit 是旧接口的页大小参数，等价于 size（size 优先）。
	Limit *int `form:"limit" binding:"omitempty,min=1,max=50"`
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
	switch {
	case q.Size != nil:
		size = *q.Size
	case q.Limit != nil:
		size = *q.Limit
	}
	items, total, err := h.svc.List(c.Request.Context(), userID, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, total, page, size)
}

func (h *handler) unreadCount(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	n, err := h.svc.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"count": n})
}

type idURI struct {
	ID int64 `uri:"id" binding:"required,gt=0"`
}

func (h *handler) markRead(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	uri, ok := httpx.BindURI[idURI](c)
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), userID, uri.ID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, true)
}

func (h *handler) readAll(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	n, err := h.svc.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, n)
}
