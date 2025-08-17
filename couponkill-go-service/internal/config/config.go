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
		UserName string `yaml:"username"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"data.redis"`
	Nacos struct {
		ServerAddr  string `yaml:"server-addr"`
		NamespaceId string `yaml:"namespace-id"`
	} `yaml:"nacos"`
	Collaboration struct {
		JavaServiceUrl   string `yaml:"java-service-url"`
		GoServiceUrl     string `yaml:"go-service-url"`
		JavaQpsThreshold int    `yaml:"java-qps-threshold"`
		GoEnabled        bool   `yaml:"go-enabled"`
		FallbackToGo     bool   `yaml:"fallback-to-go"`
	} `yaml:"collaboration"`
}

// Load 加载配置：优先从Nacos读取，失败后尝试本地配置，最后使用默认值
func Load() (*Config, error) {
	var cfg Config

	// 1. 获取Nacos连接配置（环境变量优先）
	nacosServerAddr := getEnvOrDefault("NACOS_SERVER_ADDR", "localhost:8848")
	nacosNamespaceId := getEnvOrDefault("NACOS_NAMESPACE_ID", "120")

	// 2. 优先从Nacos加载配置
	nacosLoaded := false
	nacosClient, err := nacosclient.NewNacosClient(nacosServerAddr, nacosNamespaceId)
	if err != nil {
		log.Printf("创建Nacos客户端失败: %v，将尝试本地配置", err)
	} else {
		// 从Nacos获取配置（使用带.yaml后缀的dataId，与Java客户端保持一致）
		content, err := nacosClient.GetConfig("go-service-dev.yaml", "DEFAULT_GROUP")
		if err != nil {
			log.Printf("从Nacos获取配置失败: %v，将尝试本地配置", err)
		} else {
			if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
				log.Printf("解析Nacos配置失败: %v，将尝试本地配置", err)
			} else {
				log.Println("成功从Nacos加载配置")
				nacosLoaded = true // 标记Nacos配置加载成功
			}
		}

		// 无论配置是否加载成功，都尝试监听配置变化（用于后续更新）
		if !nacosLoaded {
			err = nacosClient.ListenConfig("go-service-dev.yaml", "DEFAULT_GROUP", func(namespace, group, dataId, data string) {
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
	}

	// 3. 如果Nacos加载失败，尝试从本地配置文件读取
	if !nacosLoaded {
		if _, err := os.Stat("config.yaml"); err == nil {
			data, err := os.ReadFile("config.yaml")
			if err != nil {
				log.Printf("读取本地配置文件失败: %v，将使用默认配置", err)
			} else {
				if err := yaml.Unmarshal(data, &cfg); err != nil {
					log.Printf("解析本地配置文件失败: %v，将使用默认配置", err)
				} else {
					log.Println("成功从本地配置文件加载配置")
				}
			}
		} else {
			log.Println("本地配置文件不存在，将使用默认配置")
		}
	}

	// 4. 设置默认值（确保所有配置项都有值）
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
	// 设置协作配置默认值
	if cfg.Collaboration.JavaServiceUrl == "" {
		cfg.Collaboration.JavaServiceUrl = "http://couponkill-coupon-service:8080/api/v1" // 默认Java服务地址
	}

	if cfg.Collaboration.JavaQpsThreshold == 0 {
		cfg.Collaboration.JavaQpsThreshold = 500
	}

	if cfg.Collaboration.GoServiceUrl == "" {
		cfg.Collaboration.GoServiceUrl = "http://localhost:8090"
	}
}

// LoadCollaborationConfig 加载服务协同配置
func (cfg *Config) LoadCollaborationConfig(content string) error {
	var collaborationConfig struct {
		Collaboration struct {
			JavaServiceUrl   string `yaml:"java-service-url"`
			GoServiceUrl     string `yaml:"go-service-url"`
			JavaQpsThreshold int    `yaml:"java-qps-threshold"`
			GoEnabled        bool   `yaml:"go-enabled"`
			FallbackToGo     bool   `yaml:"fallback-to-go"`
		} `yaml:"service协同"`
	}

	if err := yaml.Unmarshal([]byte(content), &collaborationConfig); err != nil {
		return err
	}

	// 将解析的配置赋值给主配置结构体
	cfg.Collaboration.JavaServiceUrl = collaborationConfig.Collaboration.JavaServiceUrl
	cfg.Collaboration.GoServiceUrl = collaborationConfig.Collaboration.GoServiceUrl
	cfg.Collaboration.JavaQpsThreshold = collaborationConfig.Collaboration.JavaQpsThreshold
	cfg.Collaboration.GoEnabled = collaborationConfig.Collaboration.GoEnabled
	cfg.Collaboration.FallbackToGo = collaborationConfig.Collaboration.FallbackToGo

	return nil
}
