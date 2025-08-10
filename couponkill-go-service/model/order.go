package model

import (
	"time"

	"gorm.io/gorm"
)

// Order 订单模型
type Order struct {
	ID         string         `json:"id" gorm:"primaryKey"`
	UserId     int64          `json:"user_id"`
	CouponId   int64          `json:"coupon_id"`
	Status     int            `json:"status"` // 1-已创建,2-已使用,3-已过期,4-已取消
	GetTime    time.Time      `json:"get_time"`
	ExpireTime time.Time      `json:"expire_time"`
	UseTime    *time.Time     `json:"use_time,omitempty"`
	CancelTime *time.Time     `json:"cancel_time,omitempty"`
	CreateTime time.Time      `json:"create_time"`
	UpdateTime time.Time      `json:"update_time"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// TableName 指定表名
func (Order) TableName() string {
	return "order"
}
