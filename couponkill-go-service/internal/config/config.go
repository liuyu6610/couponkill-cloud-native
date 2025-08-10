package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Seckill struct {
		MaxConcurrency int    `yaml:"max-concurrency"`
		RedisPrefix    string `yaml:"redis-key-prefix"`
		ValidDays      int    `yaml:"valid-days"`
		// 新增redis嵌套层级，匹配Nacos配置
		Redis struct {
			StockKeyPrefix string `yaml:"stock-key-prefix"` // 对应seckill.redis.stock-key-prefix
		} `yaml:"redis"`
	} `yaml:"seckill"`
	Mysql struct {
		DSN string `yaml:"dsn"`
	} `yaml:"mysql"`
	Redis struct {
		Addr     string `yaml:"host"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"data.redis"`
}

// Load 加载配置
func Load() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
