package model

// Coupon 优惠券模型，包含核心字段
type Coupon struct {
	ID        int64 `json:"id"`         // 优惠券ID
	Type      int   `json:"type"`       // 类型：1-普通券，2-秒杀券
	Status    int   `json:"status"`     // 状态：0-无效，1-有效
	Stock     int64 `json:"stock"`      // 总库存
	ValidDays int   `json:"valid_days"` // 有效期（天）
}
