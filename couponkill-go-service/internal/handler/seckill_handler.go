// internal/handler/seckill_handler.go
package handler

import (
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
	var req struct {
		UserID   int64 `json:"user_id" binding:"required"`
		CouponID int64 `json:"coupon_id" binding:"required"`
	}

	// 参数校验
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: " + err.Error(),
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
