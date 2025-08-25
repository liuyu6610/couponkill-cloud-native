package sharding

import (
	"fmt"
	"strconv"
	"strings"
)

// OrderSharding 订单分库分表策略
type OrderSharding struct{}

// GetDatabaseIndex 获取订单数据库索引 (user_id % 4)
func (s *OrderSharding) GetDatabaseIndex(userID int64) int {
	return int(userID % 4)
}

// GetTableIndex 获取订单表索引 (user_id % 16)
func (s *OrderSharding) GetTableIndex(userID int64) int {
	return int(userID % 16)
}

// GetTableName 获取订单表名
func (s *OrderSharding) GetTableName(userID int64) string {
	return fmt.Sprintf("order_%d", s.GetTableIndex(userID))
}

// GetDatabaseName 获取订单数据库名
func (s *OrderSharding) GetDatabaseName(userID int64) string {
	return fmt.Sprintf("order_db_%d", s.GetDatabaseIndex(userID))
}

// GetDataSourceName 获取数据源名称
func (s *OrderSharding) GetDataSourceName(userID int64) string {
	return fmt.Sprintf("order-db-%d", s.GetDatabaseIndex(userID))
}

// CouponSharding 优惠券分库分表策略
type CouponSharding struct{}

// GetDatabaseIndex 获取优惠券数据库索引 (virtual_id后缀 % 4)
func (s *CouponSharding) GetDatabaseIndex(virtualID string) int {
	parts := strings.Split(virtualID, "_")
	if len(parts) < 2 {
		return 0
	}

	suffix, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}

	return suffix % 4
}

// GetTableIndex 获取优惠券表索引 (virtual_id后缀 % 16)
func (s *CouponSharding) GetTableIndex(virtualID string) int {
	parts := strings.Split(virtualID, "_")
	if len(parts) < 2 {
		return 0
	}

	suffix, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}

	return suffix % 16
}

// GetTableName 获取优惠券表名
func (s *CouponSharding) GetTableName(virtualID string) string {
	return fmt.Sprintf("coupon_%d", s.GetTableIndex(virtualID))
}

// GetDatabaseName 获取优惠券数据库名
func (s *CouponSharding) GetDatabaseName(virtualID string) string {
	return fmt.Sprintf("coupon_db_%d", s.GetDatabaseIndex(virtualID))
}

// GetDataSourceName 获取数据源名称
func (s *CouponSharding) GetDataSourceName(virtualID string) string {
	return fmt.Sprintf("coupon-db-%d", s.GetDatabaseIndex(virtualID))
}

// UserSharding 用户分库分表策略
type UserSharding struct{}

// GetDatabaseIndex 获取用户数据库索引 (user_id % 2)
func (s *UserSharding) GetDatabaseIndex(userID int64) int {
	return int(userID % 2)
}

// GetDataSourceName 获取数据源名称
func (s *UserSharding) GetDataSourceName(userID int64) string {
	return fmt.Sprintf("user-db-%d", s.GetDatabaseIndex(userID))
}
