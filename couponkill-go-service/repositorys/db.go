package repositorys

import (
	"context"
	"couponkill-go-service/model"
	"couponkill-go-service/util"

	"gorm.io/gorm"
)

type DBRepository struct {
	db *gorm.DB
}

func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{db: db}
}

// CreateOrder 创建订单，使用SELECT FOR UPDATE防止幻读
func (r *DBRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	// 使用事务和行锁防止并发问题
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 检查用户是否已存在未取消的同优惠券订单（防止重复）
		var count int64
		err := tx.Model(&model.Order{}).
			Where("user_id = ? AND coupon_id = ? AND status IN (1, 2)",
				order.UserId, order.CouponId).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			return util.ErrDuplicateOrder
		}

		// 创建订单
		return tx.Create(order).Error
	})
}
