// Package handler 是 user 服务的 HTTP 适配层：绑定请求、调用 service、渲染统一信封。
//
// 身份只来自 httpx.RequireGatewayIdentity 验签后的结果，handler 从不直接读取 X-User-* 头；
// 口令永远不接受查询串，也不会出现在任何响应里。
package handler

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// Service 是 handler 依赖的业务接口，*service.Service 满足。
type Service interface {
	// Register 注册普通用户。
	Register(ctx context.Context, in service.RegisterInput) (service.User, error)
	// Login 校验凭证并签发令牌。
	Login(ctx context.Context, in service.LoginInput) (service.LoginResult, error)
	// Logout 吊销当前令牌。
	Logout(ctx context.Context, id authx.Identity, rawToken string) error
	// GetUser 按 ID 取用户视图。
	GetUser(ctx context.Context, id int64) (service.User, error)
	// GenerateBatch 批量创建压测用户。
	GenerateBatch(ctx context.Context, in service.BatchInput) (service.BatchResult, error)
}

// Options 是 Mount 的参数。
type Options struct {
	// Service 是业务实现。
	Service Service
	// Signer 验证网关签名的身份头，保护 logout/profile/batch。
	Signer *authx.GatewaySigner
	// InternalToken 是 /internal/v1/** 所需的 X-Internal-Token。
	InternalToken string
}

// Handler 持有 service，方法即各端点的 gin 处理函数。
type Handler struct {
	svc Service
}

// Mount 在 r 上注册 user 服务的全部路由：
//
//	POST /api/v1/user/register         公开
//	POST /api/v1/user/login            公开
//	POST /api/v1/user/logout           网关身份
//	GET  /api/v1/user/profile          网关身份
//	POST /api/v1/user/batch/generate   网关身份 + admin
//	GET  /internal/v1/users/:id        内部令牌
func Mount(r gin.IRouter, o Options) {
	h := &Handler{svc: o.Service}

	v1 := r.Group("/api/v1/user")
	v1.POST("/register", rejectQueryPassword, h.Register)
	v1.POST("/login", rejectQueryPassword, h.Login)

	authed := v1.Group("", httpx.RequireGatewayIdentity(o.Signer))
	authed.POST("/logout", h.Logout)
	authed.GET("/profile", h.Profile)
	authed.POST("/batch/generate", httpx.RequireRole(authx.RoleAdmin), rejectQueryPassword, h.BatchGenerate)

	r.Group("", httpx.RequireInternalToken(o.InternalToken)).GET(internalapi.PathUserRoute, h.InternalUser)
}

// queryPasswordMessage 是口令出现在 URL 时的统一拒绝文案（沿用 Java）。
const queryPasswordMessage = "请勿在URL中传递密码，请通过请求体提交"

// rejectQueryPassword 拒绝查询串里带口令的请求：URL 会进入网关/Ingress/浏览器历史等访问日志。
// 参数名先做 URL 解码再不区分大小写比较，使 pass%77ord 之类的变体同样被拒。
func rejectQueryPassword(c *gin.Context) {
	for _, pair := range strings.Split(c.Request.URL.RawQuery, "&") {
		rawName, _, _ := strings.Cut(pair, "=")
		name, err := url.QueryUnescape(rawName)
		if err != nil {
			name = rawName
		}
		if strings.EqualFold(name, "password") {
			httpx.Fail(c, apperr.InvalidRequest.WithMessage(queryPasswordMessage))
			return
		}
	}
	c.Next()
}

// userView 是对外的用户 JSON，字段名与前端 types/api.ts 的 UserInfo 对齐；刻意没有任何口令字段。
type userView struct {
	ID           int64      `json:"id,string"`
	Username     string     `json:"username"`
	Phone        string     `json:"phone,omitempty"`
	Email        string     `json:"email,omitempty"`
	Role         string     `json:"role"`
	Status       int        `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastActiveAt *time.Time `json:"lastActiveAt,omitempty"`
}

func viewOf(u service.User) userView {
	return userView{
		ID: u.ID, Username: u.Username, Phone: u.Phone, Email: u.Email, Role: u.Role, Status: u.Status,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, LastActiveAt: u.LastActiveAt,
	}
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
}

// Register 处理 POST /api/v1/user/register。
func (h *Handler) Register(c *gin.Context) {
	req, ok := httpx.BindJSON[registerRequest](c)
	if !ok {
		return
	}
	u, err := h.svc.Register(c.Request.Context(), service.RegisterInput{
		Username: req.Username, Password: req.Password, Phone: req.Phone,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, viewOf(u))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Roles     []string  `json:"roles"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Login 处理 POST /api/v1/user/login。响应带 Cache-Control: no-store，防止令牌被中间缓存。
func (h *Handler) Login(c *gin.Context) {
	req, ok := httpx.BindJSON[loginRequest](c)
	if !ok {
		return
	}
	res, err := h.svc.Login(c.Request.Context(), service.LoginInput{Username: req.Username, Password: req.Password})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	httpx.OK(c, loginResponse{
		Token: res.Token, UserID: strconv.FormatInt(res.User.ID, 10), Username: res.User.Username,
		Roles: res.Roles, ExpiresAt: res.ExpiresAt.UTC(),
	})
}

// Logout 处理 POST /api/v1/user/logout：吊销 Authorization 里的当前令牌。
func (h *Handler) Logout(c *gin.Context) {
	id, ok := httpx.IdentityFrom(c)
	if !ok {
		httpx.Fail(c, apperr.AuthFail)
		return
	}
	if err := h.svc.Logout(c.Request.Context(), id, bearerToken(c.GetHeader("Authorization"))); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

// bearerToken 取出 "Bearer <token>" 里的令牌；scheme 不区分大小写，格式不符返回空串。
func bearerToken(header string) string {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// Profile 处理 GET /api/v1/user/profile：只返回网关确认的当前用户。
func (h *Handler) Profile(c *gin.Context) {
	id, ok := httpx.IdentityFrom(c)
	if !ok {
		httpx.Fail(c, apperr.AuthFail)
		return
	}
	u, err := h.svc.GetUser(c.Request.Context(), id.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, viewOf(u))
}

type batchRequest struct {
	Prefix   string `json:"prefix"`
	Start    int    `json:"start"`
	Count    int    `json:"count"`
	Password string `json:"password"`
}

type batchResponse struct {
	Requested int `json:"requested"`
	Created   int `json:"created"`
	Existing  int `json:"existing"`
}

// BatchGenerate 处理 POST /api/v1/user/batch/generate（admin）：压测造用户，参数只认 JSON 请求体。
func (h *Handler) BatchGenerate(c *gin.Context) {
	req, ok := httpx.BindJSON[batchRequest](c)
	if !ok {
		return
	}
	res, err := h.svc.GenerateBatch(c.Request.Context(), service.BatchInput{
		Prefix: req.Prefix, Start: req.Start, Count: req.Count, Password: req.Password,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, batchResponse{Requested: res.Requested, Created: res.Created, Existing: res.Existing})
}

type userURI struct {
	ID int64 `uri:"id" binding:"required,min=1"`
}

// InternalUser 处理 GET /internal/v1/users/:id，返回 contracts/internalapi.UserInfo。
func (h *Handler) InternalUser(c *gin.Context) {
	uri, ok := httpx.BindURI[userURI](c)
	if !ok {
		return
	}
	u, err := h.svc.GetUser(c.Request.Context(), uri.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, internalapi.UserInfo{ID: u.ID, Username: u.Username, Role: u.Role, Status: u.Status})
}
