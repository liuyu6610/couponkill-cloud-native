package repository

import (
	"context"
	"couponkill-go-service/pkg/sharding"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"couponkill-go-service/internal/model"

	"github.com/lib/pq"
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
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", tableName)
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
		"SELECT COUNT(1) FROM %s"+
			" WHERE user_id = $1 AND coupon_id = $2 "+
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

// isDuplicateEntryError 检查是否为唯一约束冲突错误（PostgreSQL SQLSTATE 23505）
func isDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}
	return false
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
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET 
		total_count = %s.total_count + EXCLUDED.total_count,
		seckill_count = %s.seckill_count + EXCLUDED.seckill_count,
		update_time = EXCLUDED.update_time
	`, tableName, tableName, tableName)

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
		SET seckill_remaining_stock = seckill_remaining_stock + $1,
		    version = version + 1,
		    update_time = $2
		WHERE id = $3 AND shard_index = $4 AND version >= 0
		  AND seckill_remaining_stock + $1 >= 0
	`, tableName)

	result, err := db.ExecContext(ctx, query, change, time.Now(), couponID, parseVirtualShardIndex(virtualID))
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

	query := fmt.Sprintf("SELECT seckill_remaining_stock FROM %s WHERE id = $1 AND shard_index = $2 AND seckill_remaining_stock > 0", tableName)
	var stockCount int
	shardIndex := parseVirtualShardIndex(virtualID)
	err = db.QueryRowContext(ctx, query, couponID, shardIndex).Scan(&stockCount)
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
		SET seckill_remaining_stock = seckill_remaining_stock - 1,
		    version = version + 1,
		    update_time = $1
		WHERE id = $2 AND shard_index = $3 AND seckill_remaining_stock > 0 AND version >= 0
	`, tableName)

	shardIndex := parseVirtualShardIndex(virtualID)

	result, err := db.ExecContext(ctx, query, time.Now(), couponID, shardIndex)
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

func parseVirtualShardIndex(virtualID string) int {
	parts := strings.Split(virtualID, "_")
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return n
}
