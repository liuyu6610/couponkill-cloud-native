// internal/repository/mysql_repo.go
package repository

import (
	"context"
	"couponkill-go-service/pkg/sharding"
	"errors"
	"fmt"
	"time"

	"couponkill-go-service/internal/model"
)

// MysqlRepository MySQL数据访问层
type MysqlRepository struct {
	multiDS        *sharding.MultiDataSource
	orderSharding  *sharding.OrderSharding
	couponSharding *sharding.CouponSharding
	userSharding   *sharding.UserSharding
}

func NewMysqlRepository(multiDS *sharding.MultiDataSource) *MysqlRepository {
	return &MysqlRepository{
		multiDS:        multiDS,
		orderSharding:  &sharding.OrderSharding{},
		couponSharding: &sharding.CouponSharding{},
		userSharding:   &sharding.UserSharding{},
	}
}

// CreateOrder 创建订单（依赖联合索引防止重复）
func (r *MysqlRepository) CreateOrder(ctx context.Context, order *model.Order) error {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.orderSharding.GetDataSourceName(order.UserID)
	tableName := r.orderSharding.GetTableName(order.UserID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s
		(id, user_id, coupon_id, virtual_id, status, get_time, expire_time, created_by_java, created_by_go, create_time, update_time, request_id, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?)
	`, tableName)

	_, err = db.ExecContext(
		ctx,
		query,
		order.ID,
		order.UserID,
		order.CouponID,
		order.VirtualID,
		order.Status,
		order.GetTime,
		order.ExpireTime,
		order.CreatedByJava,
		order.CreatedByGo,
		order.RequestID,
		order.Version,
	)

	// 捕获唯一索引冲突错误（并发场景下的重复提交）
	if err != nil {
		return errors.New("创建订单失败：" + err.Error())
	}
	return nil
}

// DeleteOrder 删除订单（用于回滚）
func (r *MysqlRepository) DeleteOrder(ctx context.Context, orderID int64, userID int64) error {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.orderSharding.GetDataSourceName(userID)
	tableName := r.orderSharding.GetTableName(userID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName)
	_, err = db.ExecContext(ctx, query, orderID)
	return err
}

// CheckUserReceived 检查用户是否已通过任一终端领取
func (r *MysqlRepository) CheckUserReceived(ctx context.Context, userID, couponID int64) (bool, error) {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.orderSharding.GetDataSourceName(userID)
	tableName := r.orderSharding.GetTableName(userID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return false, fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf(
		"SELECT COUNT(1) FROM `%s`"+
			" WHERE user_id = ? AND coupon_id = ? "+
			" AND status IN (1, 2) "+
			"AND (created_by_java = 1 OR created_by_go = 1)", tableName)

	var count int
	err = db.QueryRowContext(ctx, query, userID, couponID).Scan(&count)
	if err != nil {
		if isDuplicateEntryError(err) {
			return false, errors.New("用户已领取优惠券")
		}
		return false, err
	}
	return count > 0, nil
}

// UpdateUserCouponCount 更新用户优惠券数量统计
func (r *MysqlRepository) UpdateUserCouponCount(ctx context.Context, userID int64, totalCountChange, seckillCountChange int) error {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.userSharding.GetDataSourceName(userID)
	tableName := "user_coupon_count"

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	// 使用ON DUPLICATE KEY UPDATE确保记录存在
	query := fmt.Sprintf(`
		INSERT INTO %s (user_id, total_count, seckill_count, normal_count, expired_count, version)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE 
		total_count = total_count + VALUES(total_count),
		seckill_count = seckill_count + VALUES(seckill_count),
		normal_count = normal_count + VALUES(normal_count),
		expired_count = expired_count + VALUES(expired_count),
		version = version + 1
	`, tableName)

	// 初始化默认值
	totalCount := 0
	seckillCount := 0
	normalCount := 0
	expiredCount := 0
	version := 0

	if totalCountChange != 0 {
		totalCount = totalCountChange
	}
	if seckillCountChange != 0 {
		seckillCount = seckillCountChange
	}
	if totalCountChange > 0 && seckillCountChange <= 0 {
		normalCount = totalCountChange
	}
	if totalCountChange < 0 && seckillCountChange < 0 {
		normalCount = totalCountChange - seckillCountChange
	}

	_, err = db.ExecContext(ctx, query, userID, totalCount, seckillCount, normalCount, expiredCount, version)
	return err
}

// UpdateCouponStock 更新优惠券库存
func (r *MysqlRepository) UpdateCouponStock(ctx context.Context, couponID int64, stockChange int) error {
	// 生成虚拟ID用于分库分表
	virtualID := fmt.Sprintf("%d_%d", couponID, couponID%16)

	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.couponSharding.GetDataSourceName(virtualID)
	tableName := r.couponSharding.GetTableName(virtualID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	// 更新库存，使用乐观锁
	query := fmt.Sprintf(`
		UPDATE %s 
		SET remaining_stock = remaining_stock + ?,
		    seckill_remaining_stock = seckill_remaining_stock + ?,
		    update_time = NOW(),
		    version = version + 1
		WHERE id = ? AND version = ?
	`, tableName)

	// 这里简化处理，实际应该先查询当前版本号
	_, err = db.ExecContext(ctx, query, stockChange, stockChange, couponID, 0)
	return err
}

// WaitForDataSync 等待数据同步完成，确保Java服务能够查询到数据
func (r *MysqlRepository) WaitForDataSync(ctx context.Context, order *model.Order) error {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.orderSharding.GetDataSourceName(order.UserID)
	tableName := r.orderSharding.GetTableName(order.UserID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	// 循环查询直到数据可访问或超时
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		query := fmt.Sprintf("SELECT COUNT(1) FROM `%s` WHERE id = ?", tableName)
		var count int
		err = db.QueryRowContext(ctx, query, order.ID).Scan(&count)
		if err == nil && count > 0 {
			// 数据已同步
			return nil
		}

		// 等待一小段时间再重试
		time.Sleep(10 * time.Millisecond)
	}

	return errors.New("数据同步超时")
}

// isDuplicateEntryError 检查是否为重复条目错误
func isDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	// MySQL重复条目错误
	return err.Error() == "Error 1062: Duplicate entry" ||
		// 兼容其他可能的错误信息格式
		(err.Error() != "" &&
			(err.Error()[:6] == "Error " ||
				err.Error()[:9] == "duplicate" ||
				err.Error()[:9] == "Duplicate"))
}
