package model

import (
	"database/sql/driver"
	"fmt"
	time "time"
)

// Order 订单模型（适配双来源字段）
type Order struct {
	ID            string     `gorm:"column:id;primaryKey" json:"id"`
	UserID        int64      `gorm:"column:user_id" json:"user_id"`
	CouponID      int64      `gorm:"column:coupon_id" json:"coupon_id"`
	Status        int        `gorm:"column:status" json:"status"` // 1-已创建,2-已使用,3-已过期,4-已取消
	GetTime       time.Time  `gorm:"column:get_time" json:"get_time"`
	ExpireTime    time.Time  `gorm:"column:expire_time" json:"expire_time"`
	UseTime       *time.Time `gorm:"column:use_time" json:"use_time,omitempty"`
	CancelTime    *time.Time `gorm:"column:cancel_time" json:"cancel_time,omitempty"`
	CreateTime    time.Time  `gorm:"column:create_time" json:"create_time"`
	UpdateTime    time.Time  `gorm:"column:update_time" json:"update_time"`
	CreatedByJava int        `gorm:"column:created_by_java" json:"created_by_java"` // 0-否,1-是
	CreatedByGo   int        `gorm:"column:created_by_go" json:"created_by_go"`     // 0-否,1-是
	RequestID     string     `gorm:"column:request_id" json:"request_id"`
	Version       int        `gorm:"column:version" json:"version"` // 固定为0（Go端创建不影响Java端标识）
}

// TableName 数据库表名
func (Order) TableName() string {
	return "order"
}

// Value 实现driver.Valuer接口
func Value(t *time.Time) (driver.Value, error) {
	if t == nil {
		return nil, nil

	}
	return t, nil
}

// Scan 实现sql.Scanner接口
func Scan(t *time.Time, value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		*t = v
		return nil
	default:
		return fmt.Errorf("cannot scan %T into time.Time", value)
	}
}
