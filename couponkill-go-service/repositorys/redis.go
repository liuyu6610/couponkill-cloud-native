package repositorys

import (
	"context"
	"couponkill-go-service/model"
	"couponkill-go-service/util"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisRepository 封装Redis相关操作
type RedisRepository struct {
	redis *redis.Client
	// 定义Redis键前缀常量，便于统一维护
	couponStockPrefix  string // 优惠券库存键前缀，如"coupon:stock:"
	userReceivedPrefix string // 用户领取记录键前缀，如"user:received:"
	userCountPrefix    string // 用户优惠券计数键前缀，如"user:coupon:count:"
}

// NewRedisRepository 创建RedisRepository实例
func NewRedisRepository(redisClient *redis.Client, redisPrefixConfig model.RedisPrefixConfig) *RedisRepository {
	return &RedisRepository{
		redis:              redisClient,
		couponStockPrefix:  redisPrefixConfig.StockKeyPrefix,
		userReceivedPrefix: "user:received:",
		userCountPrefix:    "user:coupon:count:",
	}
}

// GetCouponStock 获取优惠券库存
func (r *RedisRepository) GetCouponStock(ctx context.Context, couponId int64) (int64, error) {
	key := fmt.Sprintf("%s%d", r.couponStockPrefix, couponId)
	return r.redis.Get(ctx, key).Int64()
}

// DeductStock 扣减优惠券库存（原子操作）
func (r *RedisRepository) DeductStock(ctx context.Context, couponId int64) (bool, error) {
	key := fmt.Sprintf("%s%d", r.couponStockPrefix, couponId)
	// 原子减1操作
	remain, err := r.redis.Decr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	// 若库存不足，回滚操作
	if remain < 0 {
		_, err := r.redis.Incr(ctx, key).Result()
		return false, err
	}
	return true, nil
}

// HasUserReceived 检查用户是否已领取优惠券
func (r *RedisRepository) HasUserReceived(ctx context.Context, userId, couponId int64) (bool, error) {
	key := fmt.Sprintf("%s%d:%d", r.userReceivedPrefix, userId, couponId)
	exists, err := r.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// SetUserReceived 记录用户领取优惠券
func (r *RedisRepository) SetUserReceived(ctx context.Context, userId, couponId int64, expire time.Duration) error {
	key := fmt.Sprintf("%s%d:%d", r.userReceivedPrefix, userId, couponId)
	return r.redis.Set(ctx, key, "1", expire).Err()
}

// IncrUserCouponCount 增加用户优惠券计数（总计数和秒杀类型计数）
func (r *RedisRepository) IncrUserCouponCount(ctx context.Context, userId int64, isSeckill bool) error {
	// 增加总计数
	totalKey := fmt.Sprintf("%s%d:total", r.userCountPrefix, userId)
	if err := r.redis.Incr(ctx, totalKey).Err(); err != nil {
		return err
	}
	// 若为秒杀优惠券，增加秒杀类型计数
	if isSeckill {
		seckillKey := fmt.Sprintf("%s%d:seckill", r.userCountPrefix, userId)
		return r.redis.Incr(ctx, seckillKey).Err()
	}
	return nil
}

// GetUserCouponCount 获取用户优惠券计数
func (r *RedisRepository) GetUserCouponCount(ctx context.Context, userId int64) (total, seckill int64, err error) {
	totalKey := fmt.Sprintf("%s%d:total", r.userCountPrefix, userId)
	total, err = r.redis.Get(ctx, totalKey).Int64()
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}
	seckillKey := fmt.Sprintf("%s%d:seckill", r.userCountPrefix, userId)
	seckill, err = r.redis.Get(ctx, seckillKey).Int64()
	if err != nil && err != redis.Nil {
		return 0, 0, err
	}
	// 处理未设置的情况（默认0）
	if err == redis.Nil {
		err = nil
	}
	return
}

// PushOrderQueue 将订单消息推入队列（异步创建订单）
func (r *RedisRepository) PushOrderQueue(ctx context.Context, orderMsg []byte) error {
	return r.redis.LPush(ctx, "order:create:queue", orderMsg).Err()
}
