package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRepository Redis缓存层
type RedisRepository struct {
	client            *redis.Client
	stockPrefix       string
	receivedKeyPrefix string // 库存键前缀，如"coupon:stock:"
}

// IncrStock 增加库存（用于回滚操作）
func (r *RedisRepository) IncrStock(ctx context.Context, couponID int64) error {
	key := r.stockPrefix + strconv.FormatInt(couponID, 10)
	return r.client.Incr(ctx, key).Err()
}

func NewRedisRepository(client *redis.Client, stockPrefix string, receivedPrefix string) *RedisRepository {
	return &RedisRepository{
		client:            client,
		stockPrefix:       stockPrefix,
		receivedKeyPrefix: receivedPrefix,
	}
}

// DeductStock 扣减库存（返回是否成功）
func (r *RedisRepository) DeductStock(ctx context.Context, couponID int64) (bool, error) {
	key := r.stockPrefix + strconv.FormatInt(couponID, 10)
	// 原子递减，确保库存不会超卖
	result, err := r.client.Decr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	// 若递减后库存为负，说明扣减失败（已售罄）
	if result < 0 {
		// 回滚库存
		r.client.Incr(ctx, key)
		return false, errors.New("库存不足")
	}
	return true, nil
}

// CheckUserReceivedCache 检查用户领取缓存（减轻DB压力）
func (r *RedisRepository) CheckUserReceivedCache(ctx context.Context, userID, couponID int64) (bool, error) {
	key := "user:received:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(couponID, 10)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil // 缓存未命中
	}
	if err != nil {
		return false, err
	}
	return val == "1", nil
}

// SetUserReceivedCache 设置用户领取缓存
func (r *RedisRepository) SetUserReceivedCache(ctx context.Context, userID, couponID int64) error {
	key := "user:received:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(couponID, 10)
	return r.client.Set(ctx, key, "1", 24*time.Hour).Err()
}
