package redisclient

import (
	"context"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient 初始化Redis连接（支持普通模式和哨兵模式）
func NewRedisClient(addr, username string, password string, db int) *redis.Client {
	// 普通模式连接
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       db,
	})
}

// NewRedisSentinelClient 初始化Redis哨兵模式连接
func NewRedisSentinelClient(sentinelAddrs []string, masterName, password string, db int) *redis.Client {
	// 哨兵模式连接
	return redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    masterName,
		SentinelAddrs: sentinelAddrs,
		Password:      password,
		DB:            db,
	})
}

// NewRedisClusterClient 初始化Redis集群模式连接
func NewRedisClusterClient(addrs []string, password string) *redis.ClusterClient {
	// 集群模式连接
	return redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    addrs,
		Password: password,
	})
}

// CheckRedisConnection 检查Redis连接状态
func CheckRedisConnection(ctx context.Context, client interface {
	Ping(ctx context.Context) *redis.StatusCmd
}) error {
	return client.Ping(ctx).Err()
}
