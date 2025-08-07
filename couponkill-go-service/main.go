// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	db       *gorm.DB
	redisCli *redis.Client
	ctx      = context.Background()
	// 用于限制并发的计数器
	concurrencyLimit int32 = 0
	maxConcurrency   int32 = 1000
)

// 优惠券结构体
type Coupon struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Type           int    `json:"type"`
	RemainingStock int    `json:"remaining_stock"`
	ValidDays      int    `json:"valid_days"`
}

// 秒杀请求
type SeckillRequest struct {
	UserId   int64 `json:"user_id" binding:"required"`
	CouponId int64 `json:"coupon_id" binding:"required"`
}

// 初始化数据库连接
func initDB() {
	dsn := "root:root@tcp(localhost:3306)/couponkill?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
}

// 初始化Redis连接
func initRedis() {
	redisCli = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	_, err := redisCli.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
}

// 检查并获取优惠券信息
func getCoupon(couponId int64) (*Coupon, bool) {
	// 先查Redis缓存
	key := fmt.Sprintf("coupon:detail:%d", couponId)
	couponJSON, err := redisCli.Get(ctx, key).Result()
	if err == nil {
		var coupon Coupon
		if err := json.Unmarshal([]byte(couponJSON), &coupon); err == nil {
			if coupon.RemainingStock > 0 {
				return &coupon, true
			}
			return &coupon, false
		}
	}

	// 缓存未命中，查数据库
	var coupon Coupon
	result := db.First(&coupon, couponId)
	if result.Error != nil || coupon.RemainingStock <= 0 {
		return nil, false
	}

	// 存入Redis缓存
	// 存入Redis缓存
	couponBytes, err := json.Marshal(coupon)
	if err != nil {
		log.Printf("Failed to marshal coupon: %v", err)
	} else {
		redisCli.Set(ctx, key, string(couponBytes), 30*time.Minute)
	}

	return &coupon, true
}

// 检查用户是否已领取优惠券
func hasUserReceived(userId, couponId int64) bool {
	key := fmt.Sprintf("user:received:%d:%d", userId, couponId)
	exists, _ := redisCli.Exists(ctx, key).Result()
	return exists > 0
}

// 检查用户优惠券数量限制
func checkUserLimit(userId int64, couponType int) bool {
	totalKey := fmt.Sprintf("user:coupon:count:%d:total", userId)
	seckillKey := fmt.Sprintf("user:coupon:count:%d:seckill", userId)

	totalCount, _ := redisCli.Get(ctx, totalKey).Int()
	seckillCount, _ := redisCli.Get(ctx, seckillKey).Int()

	// 总优惠券限制15个
	if totalCount >= 15 {
		return false
	}

	// 秒杀优惠券限制5个
	if couponType == 2 && seckillCount >= 5 {
		return false
	}

	return true
}

// 扣减库存
func deductStock(couponId int64) bool {
	stockKey := fmt.Sprintf("coupon:stock:%d", couponId)

	// 使用Redis原子操作扣减库存
	remain, err := redisCli.Decr(ctx, stockKey).Result()
	if err != nil {
		return false
	}

	if remain < 0 {
		// 库存不足，回滚
		redisCli.Incr(ctx, stockKey)
		return false
	}

	// 发送消息到RocketMQ，异步更新数据库
	sendStockUpdateMessage(couponId, -1)
	return true
}

// 发送库存更新消息
func sendStockUpdateMessage(couponId int64, quantity int) {
	// 实际项目中这里会发送消息到RocketMQ
	// 简化实现，仅打印日志
	log.Printf("Send stock update message: couponId=%d, quantity=%d", couponId, quantity)
}

// 秒杀处理函数
func seckillHandler(c *gin.Context) {
	// 检查并发限制
	if !atomic.CompareAndSwapInt32(&concurrencyLimit, concurrencyLimit, concurrencyLimit+1) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 503, "message": "系统繁忙，请稍后再试"})
		return
	}
	defer atomic.AddInt32(&concurrencyLimit, -1)

	if concurrencyLimit > maxConcurrency {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 503, "message": "当前请求过多，请稍后再试"})
		return
	}

	var req SeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}

	// 1. 检查优惠券是否存在且有库存
	coupon, hasStock := getCoupon(req.CouponId)
	if coupon == nil {
		c.JSON(http.StatusOK, gin.H{"code": 2001, "message": "优惠券不存在"})
		return
	}
	if !hasStock {
		c.JSON(http.StatusOK, gin.H{"code": 2002, "message": "优惠券已抢完"})
		return
	}

	// 2. 检查用户是否已领取
	if hasUserReceived(req.UserId, req.CouponId) {
		c.JSON(http.StatusOK, gin.H{"code": 3003, "message": "不可重复秒杀"})
		return
	}

	// 3. 检查用户优惠券数量限制
	if !checkUserLimit(req.UserId, coupon.Type) {
		c.JSON(http.StatusOK, gin.H{"code": 2003, "message": "优惠券数量已达上限"})
		return
	}

	// 4. 扣减库存
	success := deductStock(req.CouponId)
	if !success {
		c.JSON(http.StatusOK, gin.H{"code": 2002, "message": "优惠券已抢完"})
		return
	}

	// 5. 发送创建订单消息到RocketMQ
	orderMsg := map[string]interface{}{
		"user_id":   req.UserId,
		"coupon_id": req.CouponId,
		"type":      coupon.Type,
	}
	msgJSON, _ := json.Marshal(orderMsg)
	redisCli.LPush(ctx, "order:create:queue", msgJSON)

	// 6. 更新用户领取记录和计数
	redisCli.Set(ctx, fmt.Sprintf("user:received:%d:%d", req.UserId, req.CouponId), "1", 24*time.Hour)
	redisCli.Incr(ctx, fmt.Sprintf("user:coupon:count:%d:total", req.UserId))
	if coupon.Type == 2 {
		redisCli.Incr(ctx, fmt.Sprintf("user:coupon:count:%d:seckill", req.UserId))
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "秒杀成功", "data": map[string]interface{}{
		"coupon_id": coupon.ID,
		"name":      coupon.Name,
	}})
}

func main() {
	// 初始化连接
	initDB()
	initRedis()

	// 设置路由
	r := gin.Default()
	r.POST("/seckill", seckillHandler)

	// 启动服务
	log.Println("Go seckill service started on :8090")
	r.Run(":8090")
}
