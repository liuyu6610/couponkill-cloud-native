package config

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"

	"couponkill-go-service/pkg/redisclient"
)

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `yaml:"host"`
	UserName string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// 哨兵模式配置
	Sentinel struct {
		Enabled    bool     `yaml:"enabled"`
		MasterName string   `yaml:"masterName"`
		Nodes      []string `yaml:"nodes"`
		Password   string   `yaml:"password"`
	} `yaml:"sentinel"`
	// 集群模式配置
	Cluster struct {
		Enabled bool     `yaml:"enabled"`
		Nodes   []string `yaml:"nodes"`
	} `yaml:"cluster"`
}

// initializeRedisConnections 初始化Redis连接
func initializeRedisConnections(cfg *Config) {
	ctx := context.Background()

	// 根据配置选择Redis连接方式
	if cfg.Middleware.Redis.Cluster.Enabled && len(cfg.Middleware.Redis.Cluster.Nodes) > 0 {
		// Redis集群模式
		log.Println("初始化Redis集群连接")
		RedisClusterClient = redisclient.NewRedisClusterClient(cfg.Middleware.Redis.Cluster.Nodes, cfg.Redis.Password)
	} else if cfg.Middleware.Redis.Sentinel.Enabled && len(cfg.Middleware.Redis.Sentinel.Nodes) > 0 {
		// Redis哨兵模式
		log.Println("初始化Redis哨兵连接")
		RedisClient = redisclient.NewRedisSentinelClient(
			cfg.Middleware.Redis.Sentinel.Nodes,
			cfg.Middleware.Redis.Sentinel.MasterName,
			cfg.Middleware.Redis.Sentinel.Password,
			cfg.Redis.DB,
		)
	} else {
		// Redis单节点模式
		log.Println("初始化Redis单节点连接")
		RedisClient = redisclient.NewRedisClient(cfg.Redis.Addr, cfg.Redis.UserName, cfg.Redis.Password, cfg.Redis.DB)
	}

	// 检查Redis连接
	var redisClient interface {
		Ping(ctx context.Context) *redis.StatusCmd
	}

	if RedisClusterClient != nil {
		redisClient = RedisClusterClient
	} else if RedisClient != nil {
		redisClient = RedisClient
	}

	if redisClient != nil {
		if err := redisclient.CheckRedisConnection(ctx, redisClient); err != nil {
			log.Printf("Redis连接检查失败: %v", err)
		} else {
			log.Println("Redis连接正常")
		}
	}
}
