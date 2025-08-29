package config

import (
	"log"
	"os"
	"time"

	"couponkill-go-service/pkg/nacosclient"
	"couponkill-go-service/pkg/sharding"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

// 全局中间件客户端
var (
	RedisClient        *redis.Client
	RedisClusterClient *redis.ClusterClient
	MySQLClient        *sharding.MultiDataSource
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
		// 单数据源配置（用于向后兼容）
		DSN      string `yaml:"dsn"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Database string `yaml:"database"`
		// 多数据源配置
		DataSources map[string]DataSourceConfig `yaml:"dataSources"`
		// 主从复制配置
		Replication struct {
			Enabled bool               `yaml:"enabled"`
			Master  DataSourceConfig   `yaml:"master"`
			Slaves  []DataSourceConfig `yaml:"slaves"`
		} `yaml:"replication"`
	} `yaml:"mysql"`
	Redis struct {
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
	Middleware struct {
		Mysql struct {
			Cluster struct {
				Enabled bool     `yaml:"enabled"`
				Nodes   []string `yaml:"nodes"`
			} `yaml:"cluster"`
			Replication struct {
				Enabled bool `yaml:"enabled"`
				Master  struct {
					Host string `yaml:"host"`
					Port int    `yaml:"port"`
				} `yaml:"master"`
				Slaves []struct {
					Host string `yaml:"host"`
					Port int    `yaml:"port"`
				} `yaml:"slaves"`
			} `yaml:"replication"`
		} `yaml:"mysql"`
		Redis struct {
			Cluster struct {
				Enabled bool     `yaml:"enabled"`
				Nodes   []string `yaml:"nodes"`
			} `yaml:"cluster"`
			Sentinel struct {
				Enabled    bool     `yaml:"enabled"`
				MasterName string   `yaml:"masterName"`
				Nodes      []string `yaml:"nodes"`
				Password   string   `yaml:"password"`
			} `yaml:"sentinel"`
		} `yaml:"redis"`
		Rocketmq struct {
			Cluster struct {
				Enabled    bool     `yaml:"enabled"`
				NameServer []string `yaml:"nameServer"`
			} `yaml:"cluster"`
		} `yaml:"rocketmq"`
		Kafka struct {
			Cluster struct {
				Enabled bool     `yaml:"enabled"`
				Brokers []string `yaml:"brokers"`
			} `yaml:"cluster"`
		} `yaml:"kafka"`
	} `yaml:"middleware"`
}

// DataSourceConfig 数据源配置
type DataSourceConfig struct {
	DSN      string `yaml:"dsn"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
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

				// 添加配置监听器
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

		// 添加中间件集群配置监听器
		err = nacosClient.ListenConfig("middleware-cluster-config.yaml", "DEFAULT_GROUP", func(namespace, group, dataId, data string) {
			log.Printf("中间件集群配置发生变化: namespace=%s, group=%s, dataId=%s", namespace, group, dataId)
			var middlewareConfig struct {
				Middleware struct {
					Mysql struct {
						Cluster struct {
							Enabled bool     `yaml:"enabled"`
							Nodes   []string `yaml:"nodes"`
						} `yaml:"cluster"`
						Replication struct {
							Enabled bool `yaml:"enabled"`
							Master  struct {
								Host string `yaml:"host"`
								Port int    `yaml:"port"`
							} `yaml:"master"`
							Slaves []struct {
								Host string `yaml:"host"`
								Port int    `yaml:"port"`
							} `yaml:"slaves"`
						} `yaml:"replication"`
					} `yaml:"mysql"`
					Redis struct {
						Cluster struct {
							Enabled bool     `yaml:"enabled"`
							Nodes   []string `yaml:"nodes"`
						} `yaml:"cluster"`
						Sentinel struct {
							Enabled    bool     `yaml:"enabled"`
							MasterName string   `yaml:"masterName"`
							Nodes      []string `yaml:"nodes"`
							Password   string   `yaml:"password"`
						} `yaml:"sentinel"`
					} `yaml:"redis"`
					Rocketmq struct {
						Cluster struct {
							Enabled    bool     `yaml:"enabled"`
							NameServer []string `yaml:"nameServer"`
						} `yaml:"cluster"`
					} `yaml:"rocketmq"`
					Kafka struct {
						Cluster struct {
							Enabled bool     `yaml:"enabled"`
							Brokers []string `yaml:"brokers"`
						} `yaml:"cluster"`
					} `yaml:"kafka"`
				} `yaml:"middleware"`
			}

			if err := yaml.Unmarshal([]byte(data), &middlewareConfig); err != nil {
				log.Printf("解析中间件集群配置失败: %v", err)
			} else {
				// 更新中间件配置
				cfg.Middleware = middlewareConfig.Middleware
				log.Println("中间件集群配置已更新")

				// 根据配置变化重新初始化中间件连接
				initializeMiddlewareConnections(&cfg)
			}
		})
		if err != nil {
			log.Printf("监听中间件集群配置失败: %v", err)
		}

		// 添加服务协同配置监听器
		err = nacosClient.ListenConfig("service-collaboration.yaml", "DEFAULT_GROUP", func(namespace, group, dataId, data string) {
			log.Printf("服务协同配置发生变化: namespace=%s, group=%s, dataId=%s", namespace, group, dataId)
			if err := cfg.LoadCollaborationConfig(data); err != nil {
				log.Printf("解析服务协同配置失败: %v", err)
			} else {
				log.Println("服务协同配置已更新")
			}
		})
		if err != nil {
			log.Printf("监听服务协同配置失败: %v", err)
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

	// 5. 定期检查配置更新
	go func() {
		if nacosClient != nil {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				content, err := nacosClient.GetConfig("go-service-dev.yaml", "DEFAULT_GROUP")
				if err != nil {
					log.Printf("定期检查配置更新失败: %v", err)
					continue
				}

				var newCfg Config
				if err := yaml.Unmarshal([]byte(content), &newCfg); err != nil {
					log.Printf("解析定期检查的配置失败: %v", err)
					continue
				}

				// 比较配置是否有变化
				if isConfigChanged(&cfg, &newCfg) {
					log.Println("检测到配置变更，正在更新...")
					cfg = newCfg
					log.Println("配置已更新")
				}
			}
		}
	}()

	return &cfg, nil
}

// LoadCollaborationConfig 加载服务协同配置
func (c *Config) LoadCollaborationConfig(content string) error {
	var collaborationConfig struct {
		Collaboration struct {
			JavaServiceUrl   string `yaml:"java-service-url"`
			GoServiceUrl     string `yaml:"go-service-url"`
			JavaQpsThreshold int    `yaml:"java-qps-threshold"`
			GoEnabled        bool   `yaml:"go-enabled"`
			FallbackToGo     bool   `yaml:"fallback-to-go"`
		} `yaml:"collaboration"`
	}

	if err := yaml.Unmarshal([]byte(content), &collaborationConfig); err != nil {
		return err
	}

	c.Collaboration.JavaServiceUrl = collaborationConfig.Collaboration.JavaServiceUrl
	c.Collaboration.GoServiceUrl = collaborationConfig.Collaboration.GoServiceUrl
	c.Collaboration.JavaQpsThreshold = collaborationConfig.Collaboration.JavaQpsThreshold
	c.Collaboration.GoEnabled = collaborationConfig.Collaboration.GoEnabled
	c.Collaboration.FallbackToGo = collaborationConfig.Collaboration.FallbackToGo

	return nil
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

// isConfigChanged 比较两个配置是否不同
func isConfigChanged(oldCfg, newCfg *Config) bool {
	// 简单比较，实际应用中可能需要更复杂的比较逻辑
	oldBytes, _ := yaml.Marshal(oldCfg)
	newBytes, _ := yaml.Marshal(newCfg)
	return string(oldBytes) != string(newBytes)
}

// initializeMiddlewareConnections 根据配置初始化中间件连接
func initializeMiddlewareConnections(cfg *Config) {
	// 初始化Redis连接
	initializeRedisConnections(cfg)

	// 初始化MySQL连接
	var err error

	// 根据配置选择MySQL连接方式
	if cfg.Middleware.Mysql.Cluster.Enabled && len(cfg.Middleware.Mysql.Cluster.Nodes) > 0 {
		// MySQL集群模式
		log.Println("初始化MySQL集群连接")
		MySQLClient, err = initializeMySQLCluster(cfg)
	} else if cfg.Middleware.Mysql.Replication.Enabled {
		// MySQL主从复制模式
		log.Println("初始化MySQL主从复制连接")
		MySQLClient, err = initializeMySQLReplication(cfg)
	} else {
		// MySQL单节点模式
		log.Println("初始化MySQL单节点连接")
		MySQLClient, err = initializeMySQLStandalone(cfg)
	}

	if err != nil {
		log.Printf("MySQL连接初始化失败: %v", err)
	} else {
		log.Println("MySQL连接初始化成功")
	}

	// TODO: 初始化其他中间件连接 (如RocketMQ, Kafka等)
}
