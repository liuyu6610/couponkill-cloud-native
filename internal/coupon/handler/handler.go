// Package handler 是 coupon 服务的 HTTP 适配层：路由、入参绑定与校验、响应 DTO。
// 业务规则全部在 service 包；本包只做协议转换，并保证 JSON 字段名与前端 types/api.ts 一致
// （camelCase、ID 一律输出为字符串、库存字段 totalStock / remainingStock）。
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// Options 是 Mount 所需的依赖。
type Options struct {
	// Service 是业务入口（必填）。
	Service *service.Service
	// Signer 校验网关签名的身份头（必填）。
	Signer *authx.GatewaySigner
	// InternalToken 是 /internal/v1/** 要求的 X-Internal-Token（必填，空串会让内部接口拒绝所有请求）。
	InternalToken string
	// InputLocation 用于解释不带时区的时间入参（如 "2026-03-01 10:00:00"）；nil 取 UTC。
	InputLocation *time.Location
}

// Handler 持有路由处理函数共用的依赖。
type Handler struct {
	svc    *service.Service
	signer *authx.GatewaySigner
	loc    *time.Location
}

// Mount 在引擎上注册 coupon 的全部路由：
//
//	公共（可匿名，管理员额外可见停用券）  GET /api/v1/coupon/available | /list | /:id
//	管理（需 admin 角色）                POST /api/v1/coupon/create | /:id/seckill-window | /:id/status | /admin/grant
//	                                     | /preheat-stock | /preheat-stock/:id，DELETE /api/v1/coupon/:id
//	内部（需 X-Internal-Token）          POST /internal/v1/stock/deduct | /release | /sync，GET /internal/v1/coupons/:id
func Mount(eng *httpx.Engine, o Options) error {
	if o.Service == nil || o.Signer == nil {
		return errors.New("coupon handler: Service 与 Signer 是必填项")
	}
	loc := o.InputLocation
	if loc == nil {
		loc = time.UTC
	}
	h := &Handler{svc: o.Service, signer: o.Signer, loc: loc}

	public := eng.Group("/api/v1/coupon", h.optionalIdentity())
	public.GET("/available", h.available)
	public.GET("/list", h.list)
	public.GET("/:id", h.get)

	admin := eng.Group("/api/v1/coupon", httpx.RequireGatewayIdentity(o.Signer), httpx.RequireRole("admin"))
	admin.POST("/create", h.create)
	admin.POST("/:id/seckill-window", h.seckillWindow)
	admin.POST("/:id/status", h.status)
	admin.DELETE("/:id", h.remove)
	admin.POST("/admin/grant", h.grant)
	admin.POST("/preheat-stock", h.preheatAll)
	admin.POST("/preheat-stock/:id", h.preheatOne)

	internal := eng.Group("", httpx.RequireInternalToken(o.InternalToken))
	internal.POST(internalapi.PathStockDeduct, h.deduct)
	internal.POST(internalapi.PathStockRelease, h.release)
	internal.POST(internalapi.PathStockSync, h.sync)
	internal.GET(internalapi.PathCouponRoute, h.info)
	return nil
}

// optionalIdentity 尝试校验网关签名的身份头：通过则把身份放进请求 ctx，缺失或无效一律按匿名处理（不拒绝）。
// 公共只读接口借此让管理员看到停用券，而匿名/普通用户看不到。
func (h *Handler) optionalIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		if id, err := h.signer.Verify(c.Request.Header); err == nil {
			c.Request = c.Request.WithContext(authx.WithIdentity(c.Request.Context(), id))
		}
		c.Next()
	}
}

// isAdmin 判断当前请求是否带有效的管理员身份。
func isAdmin(c *gin.Context) bool {
	id, ok := authx.IdentityFrom(c.Request.Context())
	return ok && id.IsAdmin()
}

// ---------- 响应 DTO ----------

// couponView 是对外的券 JSON，字段名与前端 Coupon 类型一致。
// totalStock 是发行总量，remainingStock 是各库存桶之和；常驻券与秒杀券都输出，没有 seckill* 库存字段
// （前端契约测试钉住了这组字段）。
type couponView struct {
	ID             idgen.ID    `json:"id"`
	Name           string      `json:"name"`
	Description    string      `json:"description,omitempty"`
	Type           int         `json:"type"`
	FaceValue      json.Number `json:"faceValue"`
	MinSpend       json.Number `json:"minSpend"`
	ValidDays      int         `json:"validDays"`
	PerUserLimit   int         `json:"perUserLimit"`
	TotalStock     int64       `json:"totalStock"`
	RemainingStock int64       `json:"remainingStock"`
	Status         int         `json:"status"`
	SeckillStartAt *time.Time  `json:"seckillStartAt"`
	SeckillEndAt   *time.Time  `json:"seckillEndAt"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
}

func toView(c service.Coupon) couponView {
	return couponView{
		ID: idgen.ID(c.ID), Name: c.Name, Description: c.Description, Type: c.Type,
		FaceValue: json.Number(c.FaceValue), MinSpend: json.Number(c.MinSpend),
		ValidDays: c.ValidDays, PerUserLimit: c.PerUserLimit, Status: c.Status,
		TotalStock: c.TotalStock, RemainingStock: c.Remaining,
		SeckillStartAt: c.SeckillStartAt, SeckillEndAt: c.SeckillEndAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func toViews(cs []service.Coupon) []couponView {
	out := make([]couponView, len(cs))
	for i, c := range cs {
		out[i] = toView(c)
	}
	return out
}

// ---------- 入参 ----------

// idURI 是路径里的券 ID。
type idURI struct {
	ID int64 `uri:"id" binding:"required,min=1"`
}

func pathID(c *gin.Context) (int64, bool) {
	u, ok := httpx.BindURI[idURI](c)
	return u.ID, ok
}

// bindFlexible 兼容两种调用方式：JSON 请求体（新前端）与查询串（现有前端用 axios params 传参，Java 的 @RequestParam 语义）。
func bindFlexible[T any](c *gin.Context) (T, bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return httpx.BindJSON[T](c)
	}
	return httpx.BindQuery[T](c)
}

// timeLayouts 是接受的时间格式：带时区的 RFC3339，或 Java 沿用的不带时区格式（按 InputLocation 解释）。
var (
	zonedLayouts   = []string{time.RFC3339Nano, time.RFC3339}
	unzonedLayouts = []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"}
)

// parseTime 解析时间入参；field 只用于错误信息。
func (h *Handler) parseTime(field, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, l := range zonedLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	for _, l := range unzonedLayouts {
		if t, err := time.ParseInLocation(l, s, h.loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, apperr.InvalidRequest.WithMessage(
		fmt.Sprintf("请求参数错误: %s 时间格式不正确（需为 RFC3339 或 yyyy-MM-dd HH:mm:ss）", field))
}

func (h *Handler) parseOptionalTime(field, s string) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := h.parseTime(field, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

type createReq struct {
	ID             *idgen.ID   `json:"id" form:"id" binding:"omitempty,min=1"`
	Name           string      `json:"name" form:"name" binding:"required"`
	Description    string      `json:"description" form:"description"`
	Type           int         `json:"type" form:"type" binding:"required,oneof=1 2"`
	FaceValue      json.Number `json:"faceValue" form:"faceValue" binding:"required"`
	MinSpend       json.Number `json:"minSpend" form:"minSpend" binding:"required"`
	ValidDays      int         `json:"validDays" form:"validDays" binding:"required"`
	PerUserLimit   int         `json:"perUserLimit" form:"perUserLimit" binding:"required"`
	TotalStock     *int64      `json:"totalStock" form:"totalStock" binding:"required"`
	SeckillStartAt string      `json:"seckillStartAt" form:"seckillStartAt"`
	SeckillEndAt   string      `json:"seckillEndAt" form:"seckillEndAt"`
}

// create 创建券（admin）。
func (h *Handler) create(c *gin.Context) {
	req, ok := bindFlexible[createReq](c)
	if !ok {
		return
	}
	start, err := h.parseOptionalTime("seckillStartAt", req.SeckillStartAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	end, err := h.parseOptionalTime("seckillEndAt", req.SeckillEndAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	cmd := service.CreateCommand{
		Name: req.Name, Description: req.Description, Type: req.Type,
		FaceValue: req.FaceValue.String(), MinSpend: req.MinSpend.String(),
		ValidDays: req.ValidDays, PerUserLimit: req.PerUserLimit, TotalStock: *req.TotalStock,
		SeckillStartAt: start, SeckillEndAt: end,
	}
	if req.ID != nil {
		cmd.ID = int64(*req.ID)
	}
	created, err := h.svc.Create(c.Request.Context(), cmd)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toView(created))
}

type windowReq struct {
	SeckillStartAt string `json:"seckillStartAt" form:"seckillStartAt" binding:"required"`
	SeckillEndAt   string `json:"seckillEndAt" form:"seckillEndAt" binding:"required"`
}

// seckillWindow 更新秒杀时间窗（admin）。
func (h *Handler) seckillWindow(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	req, ok := bindFlexible[windowReq](c)
	if !ok {
		return
	}
	start, err := h.parseTime("seckillStartAt", req.SeckillStartAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	end, err := h.parseTime("seckillEndAt", req.SeckillEndAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	updated, err := h.svc.SetSeckillWindow(c.Request.Context(), id, start, end)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toView(updated))
}

type statusReq struct {
	Status *int `json:"status" form:"status" binding:"required"`
}

// status 更新券状态（admin），data 为受影响的券数。
func (h *Handler) status(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	req, ok := bindFlexible[statusReq](c)
	if !ok {
		return
	}
	n, err := h.svc.SetStatus(c.Request.Context(), id, *req.Status)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, n)
}

// remove 删除券（admin），data 为删除的券数（不存在时幂等返回 0）。
func (h *Handler) remove(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	n, err := h.svc.Delete(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, n)
}

// available 返回当前可购买的券（数组，前端直接遍历）。
func (h *Handler) available(c *gin.Context) {
	items, err := h.svc.Available(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toViews(items))
}

type listReq struct {
	Page *int `form:"page" binding:"omitempty,min=1"`
	Size *int `form:"size" binding:"omitempty,min=1,max=100"`
}

// list 分页返回券：管理员含停用券，其余只含启用券。
func (h *Handler) list(c *gin.Context) {
	req, ok := httpx.BindQuery[listReq](c)
	if !ok {
		return
	}
	page, size := 0, 0 // 0 表示取服务默认值（page=1、size=20）
	if req.Page != nil {
		page = *req.Page
	}
	if req.Size != nil {
		size = *req.Size
	}
	p, err := h.svc.List(c.Request.Context(), page, size, isAdmin(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, toViews(p.Items), p.Total, p.Page, p.Size)
}

// get 返回一张券：停用券只有管理员看得到，其余人得到 404。
func (h *Handler) get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	cp, err := h.svc.Get(c.Request.Context(), id, isAdmin(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toView(cp))
}

type grantReq struct {
	CouponID  idgen.ID `json:"couponId" binding:"required,min=1"`
	UserID    idgen.ID `json:"userId" binding:"required,min=1"`
	RequestID string   `json:"requestId" binding:"max=128"`
}

type grantResp struct {
	RequestID      string   `json:"requestId"`
	OrderID        idgen.ID `json:"orderId"`
	AlreadyGranted bool     `json:"alreadyGranted"`
}

// grant 管理员给用户发券（admin）。
func (h *Handler) grant(c *gin.Context) {
	req, ok := httpx.BindJSON[grantReq](c)
	if !ok {
		return
	}
	res, err := h.svc.Grant(c.Request.Context(), service.GrantCommand{
		CouponID: int64(req.CouponID), UserID: int64(req.UserID), RequestID: req.RequestID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, grantResp{RequestID: res.RequestID, OrderID: idgen.ID(res.OrderID), AlreadyGranted: res.AlreadyGranted})
}

// preheatAll 全量预热启用中的秒杀券（admin）：把计数器对齐到 DB 剩余量；部分失败返回 503 并带成功/失败数。
func (h *Handler) preheatAll(c *gin.Context) {
	sum, err := h.svc.PreheatAll(c.Request.Context(), true)
	if err != nil {
		httpx.Fail(c, apperr.New(apperr.CodeSystemBusy, http.StatusServiceUnavailable,
			fmt.Sprintf("库存预热未全部成功：成功 %d / 共 %d，失败 %d", sum.Preheated, sum.Total, sum.Failed)).WithCause(err))
		return
	}
	httpx.OK(c, sum)
}

type preheatResp struct {
	CouponID idgen.ID `json:"couponId"`
	Stock    int64    `json:"stock"`
}

// preheatOne 预热单张秒杀券（admin）。
func (h *Handler) preheatOne(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	res, err := h.svc.PreheatOne(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, preheatResp{CouponID: idgen.ID(res.CouponID), Stock: res.Stock})
}

// ---------- 内部接口 ----------

// deduct 实现 POST /internal/v1/stock/deduct。
func (h *Handler) deduct(c *gin.Context) {
	req, ok := httpx.BindJSON[internalapi.DeductRequest](c)
	if !ok {
		return
	}
	res, err := h.svc.Deduct(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// release 实现 POST /internal/v1/stock/release。
func (h *Handler) release(c *gin.Context) {
	req, ok := httpx.BindJSON[internalapi.ReleaseRequest](c)
	if !ok {
		return
	}
	res, err := h.svc.Release(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// sync 实现 POST /internal/v1/stock/sync。
func (h *Handler) sync(c *gin.Context) {
	req, ok := httpx.BindJSON[internalapi.SyncRequest](c)
	if !ok {
		return
	}
	res, err := h.svc.Sync(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// info 实现 GET /internal/v1/coupons/:id。
func (h *Handler) info(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	res, err := h.svc.Info(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}
