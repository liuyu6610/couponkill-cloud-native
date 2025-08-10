package redisclient

import (
	"github.com/go-redis/redis/v8"
)

// NewRedisClient 初始化Redis连接
func NewRedisClient(addr, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}
