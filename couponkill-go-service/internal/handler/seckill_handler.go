package handler

import (
	"encoding/json"
	"io"
	"net/http"

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

// HandleSeckill 处理秒杀请求
func (h *SeckillHandler) HandleSeckill(c *gin.Context) {
	// 检查Content-Type
	contentType := c.GetHeader("Content-Type")
	if contentType != "" && contentType != "application/json" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Content-Type必须为application/json",
		})
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "读取请求体失败: " + err.Error(),
		})
		return
	}

	// 检查请求体是否为空
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求体不能为空",
		})
		return
	}

	// 解析JSON
	var req struct {
		UserID   int64 `json:"user_id" binding:"required"`
		CouponID int64 `json:"coupon_id" binding:"required"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "JSON解析失败: " + err.Error(),
			"details": string(body), // 返回原始数据便于调试
		})
		return
	}

	// 参数校验
	if req.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "用户ID必须大于0",
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

	// 执行秒杀
	success, err := h.service.ProcessSeckill(c.Request.Context(), req.UserID, req.CouponID, h.validDays)
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
	} else {
		c.JSON(http.StatusOK, gin.H{
			"code":    403,
			"message": "秒杀失败（已抢完或重复参与）",
		})
	}
}
