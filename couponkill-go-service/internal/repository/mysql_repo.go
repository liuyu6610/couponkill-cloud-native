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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		order.CreateTime,
		order.UpdateTime,
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
func (r *MysqlRepository) DeleteOrder(ctx context.Context, orderID string, userID int64) error {
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

// UpdateUserCouponCount 更新用户优惠券数量统计
func (r *MysqlRepository) UpdateUserCouponCount(ctx context.Context, userID int64, totalChange int, seckillChange int) error {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.userSharding.GetDataSourceName(userID)
	tableName := "user_coupon_count" // 用户优惠券数量表不分表

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (user_id, total_count, seckill_count, update_time) 
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE 
		total_count = total_count + VALUES(total_count),
		seckill_count = seckill_count + VALUES(seckill_count),
		update_time = VALUES(update_time)
	`, tableName)

	_, err = db.ExecContext(ctx, query, userID, totalChange, seckillChange, time.Now())
	return err
}

// UpdateCouponStock 更新优惠券库存
func (r *MysqlRepository) UpdateCouponStock(ctx context.Context, couponID int64, change int) error {
	// 根据分库分表规则确定数据源和表名
	virtualID := fmt.Sprintf("%d_%d", couponID, couponID%16)
	dataSourceName := r.couponSharding.GetDataSourceName(virtualID)
	tableName := r.couponSharding.GetTableName(virtualID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s 
		SET stock_count = stock_count + ?,
		    version = version + 1,
		    update_time = ?
		WHERE id = ? AND version >= 0
	`, tableName)

	result, err := db.ExecContext(ctx, query, change, time.Now(), couponID)
	if err != nil {
		return err
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("更新优惠券库存失败：可能由于版本冲突或优惠券不存在")
	}

	return nil
}

// CheckShardStock 检查特定分片的库存
func (r *MysqlRepository) CheckShardStock(ctx context.Context, couponID int64, virtualID string) (bool, error) {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.couponSharding.GetDataSourceName(virtualID)
	tableName := r.couponSharding.GetTableName(virtualID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return false, fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf("SELECT stock_count FROM %s WHERE id = ? AND stock_count > 0", tableName)
	var stockCount int
	err = db.QueryRowContext(ctx, query, couponID).Scan(&stockCount)
	if err != nil {
		// 如果查询出错或者没有找到记录，认为库存不足
		return false, nil
	}

	return stockCount > 0, nil
}

// DeductShardStock 扣减特定分片的库存
func (r *MysqlRepository) DeductShardStock(ctx context.Context, couponID int64, virtualID string) (bool, error) {
	// 根据分库分表规则确定数据源和表名
	dataSourceName := r.couponSharding.GetDataSourceName(virtualID)
	tableName := r.couponSharding.GetTableName(virtualID)

	// 获取对应的数据源
	db, err := r.multiDS.GetDataSource(dataSourceName)
	if err != nil {
		return false, fmt.Errorf("获取数据源失败: %v", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s 
		SET stock_count = stock_count - 1,
		    version = version + 1,
		    update_time = ?
		WHERE id = ? AND stock_count > 0 AND version >= 0
	`, tableName)

	result, err := db.ExecContext(ctx, query, time.Now(), couponID)
	if err != nil {
		return false, err
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

// WaitForDataSync 等待数据同步到ShardingSphere
func (r *MysqlRepository) WaitForDataSync(ctx context.Context, order *model.Order) error {
	// 简化处理，实际项目中可能需要更复杂的同步确认机制
	time.Sleep(100 * time.Millisecond)
	return nil
}
