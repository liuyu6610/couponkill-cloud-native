package model

import (
	"time"
)

// Coupon 优惠券模型，包含核心字段
type Coupon struct {
	ID                    int64     `json:"id"`                      // 优惠券ID
	Type                  int       `json:"type"`                    // 类型：1-普通券，2-秒杀券
	Status                int       `json:"status"`                  // 状态：0-无效，1-有效
	Stock                 int64     `json:"stock"`                   // 总库存
	RemainingStock        int64     `json:"remaining_stock"`         // 剩余库存
	SeckillTotalStock     int64     `json:"seckill_total_stock"`     // 秒杀总库存（仅秒杀类型有效）
	SeckillRemainingStock int64     `json:"seckill_remaining_stock"` // 秒杀剩余库存
	ValidDays             int       `json:"valid_days"`              // 有效期（天）
	CreateTime            time.Time `json:"create_time"`             // 创建时间
	UpdateTime            time.Time `json:"update_time"`             // 更新时间
}

// IsSeckillCoupon 判断是否为秒杀优惠券
func (c *Coupon) IsSeckillCoupon() bool {
	return c.Type == 2
}

// IsRegularCoupon 判断是否为普通优惠券
func (c *Coupon) IsRegularCoupon() bool {
	return c.Type == 1
}
