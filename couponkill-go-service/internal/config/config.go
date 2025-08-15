package config

import (
	"log"
	"os"

	"couponkill-go-service/pkg/nacosclient"

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
	Nacos struct {
		ServerAddr  string `yaml:"server-addr"`
		NamespaceId string `yaml:"namespace-id"`
	} `yaml:"nacos"`
}

// Load 加载配置
func Load() (*Config, error) {
	var cfg Config

	// 首先尝试从环境变量获取Nacos配置
	nacosServerAddr := getEnvOrDefault("NACOS_SERVER_ADDR", "localhost:8848")
	nacosNamespaceId := getEnvOrDefault("NACOS_NAMESPACE_ID", "c6504e82-cdbf-41e6-922a-53341dbdc9d2")

	// 如果环境变量中没有Nacos配置，则尝试从本地配置文件读取
	if nacosServerAddr == "localhost:8848" && nacosNamespaceId == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			data, err := os.ReadFile("config.yaml")
			if err != nil {
				log.Printf("读取本地配置文件失败: %v，将使用默认配置", err)
			} else {
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					log.Printf("解析本地配置文件失败: %v，将使用默认配置", err)
				}
			}
		}
	}

	// 尝试从Nacos加载配置
	nacosClient, err := nacosclient.NewNacosClient(nacosServerAddr, nacosNamespaceId)
	if err != nil {
		log.Printf("创建Nacos客户端失败: %v，将使用本地配置或默认配置", err)
	} else {
		// 从Nacos获取配置
		content, err := nacosClient.GetConfig("go-service-dev", "DEFAULT_GROUP")
		if err != nil {
			log.Printf("从Nacos获取配置失败: %v，将使用本地配置或默认配置", err)
		} else {
			if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
				log.Printf("解析Nacos配置失败: %v，将使用本地配置或默认配置", err)
			} else {
				log.Println("成功从Nacos加载配置")
			}
		}

		// 监听配置变化
		err = nacosClient.ListenConfig("go-service-dev", "DEFAULT_GROUP", func(namespace, group, dataId, data string) {
			log.Printf("配置发生变化: namespace=%s, group=%s, dataId=%s", namespace, group, dataId)
			var newCfg Config
			if err := yaml.Unmarshal([]byte(data), &newCfg); err != nil {
				log.Printf("解析变更的配置失败: %v", err)
			} else {
				cfg = newCfg // 更新配置
				log.Println("配置已更新")
			}
		})
		if err != nil {
			log.Printf("监听Nacos配置失败: %v", err)
		}
	}

	// 设置默认值
	setDefaultValues(&cfg)

	return &cfg, nil
}

// getEnvOrDefault 获取环境变量，如果不存在则返回默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// setDefaultValues 设置默认值
func setDefaultValues(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8090
	}

	if cfg.Seckill.ValidDays == 0 {
		cfg.Seckill.ValidDays = 7
	}

	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "116.62.178.91:6379"
	}

	if cfg.Mysql.DSN == "" {
		cfg.Mysql.DSN = "root:root@tcp(localhost:3306)/console?charset=utf8mb4&parseTime=True&loc=Local"
	}

	if cfg.Seckill.Redis.StockKeyPrefix == "" {
		cfg.Seckill.Redis.StockKeyPrefix = "coupon:stock:"
	}
}
