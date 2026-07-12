package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"couponkill-go-service/internal/service"
)

// SeckillHandler 秒杀处理器
type SeckillHandler struct {
	service   *service.SeckillService
	validDays int // 优惠券有效期（天）
}

func NewSeckillHandler(service *service.SeckillService, validDays int) *SeckillHandler {
	return &SeckillHandler{
		service:   service,
		validDays: validDays,
	}
}

// HandleSeckill 处理秒杀请求。
// 身份优先取网关注入的 X-User-Id（防 body 伪造）；无头时回退 body（供集群内 Feign）。
func (h *SeckillHandler) HandleSeckill(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "application/json") {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Content-Type必须为application/json",
		})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "读取请求体失败: " + err.Error(),
		})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求体不能为空",
		})
		return
	}

	var req struct {
		UserID   int64 `json:"user_id"`
		CouponID int64 `json:"coupon_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "JSON解析失败: " + err.Error(),
			"details": string(body),
		})
		return
	}

	userID := resolveUserID(c, req.UserID)
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "缺少有效用户身份（X-User-Id 或 user_id）",
		})
		return
	}
	if req.CouponID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "优惠券ID必须大于0",
		})
		return
	}

	success, err := h.service.ProcessSeckill(c.Request.Context(), userID, req.CouponID, h.validDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "秒杀失败: " + err.Error(),
		})
		return
	}

	if success {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "秒杀成功",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    403,
		"message": "秒杀失败（已抢完或重复参与）",
	})
}

func resolveUserID(c *gin.Context, bodyUserID int64) int64 {
	header := c.GetHeader("X-User-Id")
	if header == "" {
		header = c.GetHeader("X-User-ID")
	}
	if header != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(header), 10, 64)
		if err == nil && id > 0 {
			return id
		}
	}
	return bodyUserID
}
