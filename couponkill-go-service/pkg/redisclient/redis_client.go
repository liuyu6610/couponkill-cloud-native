package redisclient

import "github.com/redis/go-redis/v9"

// NewRedisClient 初始化Redis连接
func NewRedisClient(addr, username string, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       db,
	})
}
