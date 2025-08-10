package handler

import (
	"couponkill-go-service/model"
	"couponkill-go-service/service"
	"couponkill-go-service/util"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SeckillHandler struct {
	seckillService *service.SeckillService
	maxConcurrency int32
	workerPool     chan struct{}
}

func NewSeckillHandler(seckillService *service.SeckillService, maxConcurrency int) *SeckillHandler {
	return &SeckillHandler{
		seckillService: seckillService,
		maxConcurrency: int32(maxConcurrency),
		workerPool:     make(chan struct{}, maxConcurrency),
	}
}

// SeckillRequest 秒杀请求参数
type SeckillRequest struct {
	UserId   int64 `json:"user_id" binding:"required"`
	CouponId int64 `json:"coupon_id" binding:"required"`
}

// HandleSeckill 处理秒杀请求
func (h *SeckillHandler) HandleSeckill(c *gin.Context) {
	// 检查并发限制
	select {
	case h.workerPool <- struct{}{}:
		defer func() { <-h.workerPool }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{
			"code":    503,
			"message": "系统繁忙，请稍后再试",
		})
		return
	}

	var req SeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误: " + err.Error(),
		})
		return
	}

	// 获取优惠券有效期
	validDays, err := h.seckillService.GetCouponValidDays(c.Request.Context(), req.CouponId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "获取优惠券信息失败",
		})
		return
	}

	// 执行秒杀逻辑
	order, err := h.seckillService.DoSeckill(c.Request.Context(), req.UserId, req.CouponId, validDays)
	if err != nil {
		code, message := 500, "秒杀失败: "+err.Error()
		if err == util.ErrOutOfStock {
			code, message = 2002, "优惠券已抢完"
		} else if err == util.ErrDuplicateOrder {
			code, message = 3003, "不可重复秒杀"
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "秒杀成功",
		"data": gin.H{
			"order_id":  order.ID,
			"coupon_id": order.CouponId,
		},
	})
}
