// internal/repository/redis_repo.go
package repository

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisRepository Redis数据访问层
type RedisRepository struct {
	client            *redis.Client
	stockKeyPrefix    string
	receivedKeyPrefix string
}

// NewRedisRepository 创建Redis仓库实例
func NewRedisRepository(client *redis.Client, stockKeyPrefix, receivedKeyPrefix string) *RedisRepository {
	return &RedisRepository{
		client:            client,
		stockKeyPrefix:    stockKeyPrefix,
		receivedKeyPrefix: receivedKeyPrefix,
	}
}

// DeductStock 扣减库存（Redis原子操作）
func (r *RedisRepository) DeductStock(ctx context.Context, couponID int64) (bool, error) {
	stockKey := fmt.Sprintf("%s%d", r.stockKeyPrefix, couponID)
	result := r.client.Decr(ctx, stockKey)
	if err := result.Err(); err != nil {
		return false, err
	}

	// 如果库存小于0，说明库存不足
	if result.Val() < 0 {
		// 恢复库存
		r.client.Incr(ctx, stockKey)
		return false, nil
	}

	return true, nil
}

// IncrStock 增加库存（用于回滚）
func (r *RedisRepository) IncrStock(ctx context.Context, couponID int64) error {
	stockKey := fmt.Sprintf("%s%d", r.stockKeyPrefix, couponID)
	return r.client.Incr(ctx, stockKey).Err()
}

// CheckStock 检查库存是否充足（不扣减）
func (r *RedisRepository) CheckStock(ctx context.Context, couponID int64) (bool, error) {
	stockKey := fmt.Sprintf("%s%d", r.stockKeyPrefix, couponID)
	result := r.client.Get(ctx, stockKey)
	if err := result.Err(); err != nil {
		return false, err
	}

	stock, err := result.Int64()
	if err != nil {
		return false, err
	}

	// 库存大于0表示有库存
	return stock > 0, nil
}

// CheckUserReceivedCache 检查用户是否已在缓存中领取
func (r *RedisRepository) CheckUserReceivedCache(ctx context.Context, userID, couponID int64) (bool, error) {
	receivedKey := fmt.Sprintf("%s%d:%d", r.receivedKeyPrefix, userID, couponID)
	result := r.client.Exists(ctx, receivedKey)
	if err := result.Err(); err != nil {
		return false, err
	}
	return result.Val() > 0, nil
}

// SetUserReceivedCache 设置用户已领取缓存
func (r *RedisRepository) SetUserReceivedCache(ctx context.Context, userID, couponID int64) error {
	receivedKey := fmt.Sprintf("%s%d:%d", r.receivedKeyPrefix, userID, couponID)
	return r.client.Set(ctx, receivedKey, "1", 0).Err()
}

// UpdateUserCouponCountCache 更新用户优惠券数量缓存
func (r *RedisRepository) UpdateUserCouponCountCache(ctx context.Context, userID int64, totalCountChange, seckillCountChange int64) error {
	// 更新总数量缓存
	totalCountKey := fmt.Sprintf("user:coupon:count:total:%d", userID)
	if totalCountChange != 0 {
		r.client.IncrBy(ctx, totalCountKey, totalCountChange)
	}

	// 更新秒杀券数量缓存
	seckillCountKey := fmt.Sprintf("user:coupon:count:seckill:%d", userID)
	if seckillCountChange != 0 {
		r.client.IncrBy(ctx, seckillCountKey, seckillCountChange)
	}

	return nil
}

// UpdateCouponStockCache 更新优惠券库存缓存
func (r *RedisRepository) UpdateCouponStockCache(ctx context.Context, couponID int64, stockChange int64) error {
	stockKey := fmt.Sprintf("%s%d", r.stockKeyPrefix, couponID)
	if stockChange != 0 {
		r.client.IncrBy(ctx, stockKey, stockChange)
	}
	return nil
}
