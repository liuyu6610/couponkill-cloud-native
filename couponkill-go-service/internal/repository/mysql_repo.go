// internal/repository/mysql_repo.go
package repository

import (
	"context"
	"database/sql"
	"errors"

	"couponkill-go-service/internal/model"
)

// MysqlRepository MySQL数据访问层
type MysqlRepository struct {
	db *sql.DB
}

func NewMysqlRepository(db *sql.DB) *MysqlRepository {
	return &MysqlRepository{db: db}
}

// CreateOrder 创建订单（依赖联合索引防止重复）
func (r *MysqlRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	query := `
		INSERT INTO order
		(id, user_id, coupon_id, status, get_time, expire_time, created_by_java, created_by_go, create_time, update_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`
	_, err := r.db.ExecContext(
		ctx,
		query,
		order.ID,
		order.UserID,
		order.CouponID,
		order.Status,
		order.GetTime,
		order.ExpireTime,
		order.CreatedByJava,
		order.CreatedByGo,
	)
	// 捕获唯一索引冲突错误（并发场景下的重复提交）
	if err != nil {
		return errors.New("创建订单失败：" + err.Error())
	}
	return nil
}

// CheckUserReceived 检查用户是否已通过任一终端领取
func (r *MysqlRepository) CheckUserReceived(ctx context.Context, userID, couponID int64) (bool, error) {
	query :=
		"SELECT COUNT(1) FROM `order`" +
			" WHERE user_id = ? AND coupon_id = ? " +
			" AND status IN (1, 2) " +
			"AND (created_by_java = 1 OR created_by_go = 1)"
	var count int
	err := r.db.QueryRowContext(ctx, query, userID, couponID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
