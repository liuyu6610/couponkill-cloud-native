package seckill

import (
	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/order/orders"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
)

// Handler 是秒杀的 HTTP 适配层，路由必须挂在 RequireGatewayIdentity 之后。
type Handler struct {
	svc *Service
}

// NewHandler 创建处理器。
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register 在 r（通常是 /api/v1/order 分组）上注册：
//
//	POST /seckill          body {"couponId":"..."}（也接受查询参数 couponId）→ 202 {requestId,status:"QUEUED"}
//	GET  /seckill/result   ?requestId=... → {status, orderId?, reason?}；非属主或不存在 404
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/seckill", h.enter)
	r.GET("/seckill/result", h.result)
}

func (h *Handler) enter(c *gin.Context) {
	id, ok := httpx.IdentityFrom(c)
	if !ok {
		httpx.Fail(c, apperr.AuthFail)
		return
	}
	couponID, ok := orders.IDParam(c, "couponId")
	if !ok {
		return
	}
	res, err := h.svc.Enter(c.Request.Context(), id.UserID, couponID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Accepted(c, events.SeckillAccepted{RequestID: res.RequestID, Status: events.StatusQueued})
}

func (h *Handler) result(c *gin.Context) {
	id, ok := httpx.IdentityFrom(c)
	if !ok {
		httpx.Fail(c, apperr.AuthFail)
		return
	}
	requestID := c.Query("requestId")
	if requestID == "" {
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: requestId 为必填项"))
		return
	}
	res, err := h.svc.Result(c.Request.Context(), id.UserID, requestID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}
