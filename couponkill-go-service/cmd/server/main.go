package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	"couponkill-go-service/internal/config"
	"couponkill-go-service/internal/handler"
	"couponkill-go-service/internal/repository"
	"couponkill-go-service/internal/service"
	"couponkill-go-service/pkg/mysqlclient"
	"couponkill-go-service/pkg/redisclient"
	"couponkill-go-service/pkg/sharding"
)

// main 是程序的入口函数，负责初始化配置、连接依赖服务、设置路由并启动HTTP服务器。
// 该函数不接受参数，也不返回任何值。
func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 2. 从Nacos获取服务协同配置
	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Nacos.NamespaceId,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
	}
	// 新增：解析Nacos服务器地址为host和port
	host, portStr, err := net.SplitHostPort(cfg.Nacos.ServerAddr)
	if err != nil {
		log.Fatalf("解析Nacos服务器地址失败: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("解析Nacos端口失败: %v", err)
	}

	// 构建服务器配置
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: host,
			Port:   uint64(port),
			Scheme: "http",
		},
	}

	// 创建配置客户端时同时传入serverConfigs和clientConfig
	configClient, err := clients.CreateConfigClient(map[string]interface{}{
		"serverConfigs": serverConfigs, // 补充服务器配置
		"clientConfig":  clientConfig,
	})
	if err != nil {
		log.Fatalf("创建Nacos配置客户端失败: %v", err)
	}

	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: "service-collaboration.yaml",
		Group:  "DEFAULT_GROUP",
	})
	if err != nil {
		log.Printf("获取服务协同配置失败: %v, 使用默认配置", err)
	} else {
		// 解析配置并应用
		if err := cfg.LoadCollaborationConfig(content); err != nil {
			log.Printf("解析服务协同配置失败: %v, 使用默认配置", err)
		}
	}

	log.Printf("配置加载成功: 端口=%d, Redis地址=%s", cfg.Server.Port, cfg.Redis.Addr)

	// 3. 初始化客户端
	multiDS, err := initializeMySQLClient(cfg)
	if err != nil {
		log.Fatalf("初始化MySQL客户端失败: %v", err)
	}
	defer multiDS.Close()

	redisCli := redisclient.NewRedisClient(cfg.Redis.Addr, cfg.Redis.UserName, cfg.Redis.Password, cfg.Redis.DB)
	defer redisCli.Close()

	// 4. 初始化依赖
	redisRepo := repository.NewRedisRepository(redisCli, cfg.Seckill.Redis.StockKeyPrefix, "user:received:")
	mysqlRepo := repository.NewMysqlRepository(multiDS)
	seckillService := service.NewSeckillService(mysqlRepo, redisRepo)
	seckillHandler := handler.NewSeckillHandler(seckillService, cfg.Seckill.ValidDays)

	// 确保在程序退出时关闭工作池
	defer func() {
		// 可以在这里添加任何清理工作
	}()

	// 5. 路由设置
	r := gin.Default()
	r.POST("/seckill", seckillHandler.HandleSeckill)

	// 6. 启动HTTP服务器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	// 在goroutine中启动服务器，以便可以捕获关闭信号
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	log.Printf("服务器启动成功，监听端口 %d", cfg.Server.Port)

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 设置超时时间以防止长时间阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("服务器强制关闭: ", err)
	}

	log.Println("服务器已退出")
}

// initializeMySQLClient 初始化MySQL客户端
func initializeMySQLClient(cfg *config.Config) (*sharding.MultiDataSource, error) {
	// 转换数据源配置格式
	convertedDataSources := make(map[string]struct{ DSN string })
	for name, dsConfig := range cfg.Mysql.DataSources {
		convertedDataSources[name] = struct{ DSN string }{DSN: dsConfig.DSN}
	}

	// 初始化MySQL客户端
	multiDS, err := mysqlclient.NewMultiMysqlClient(
		cfg.Mysql.DSN,
		convertedDataSources,
	)
	if err != nil {
		return nil, fmt.Errorf("初始化MySQL客户端失败: %v", err)
	}

	// 将全局MySQL客户端变量设置为新创建的客户端
	config.MySQLClient = multiDS
	return multiDS, nil
}

// monitorMySQLClient 监控MySQL客户端状态并在需要时重新初始化
func monitorMySQLClient(cfg *config.Config, currentDS *sharding.MultiDataSource) {
	// 定期检查MySQL客户端是否需要重新初始化
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// 检查全局MySQL客户端是否为nil，表示需要重新初始化
		if config.MySQLClient == nil {
			log.Println("检测到MySQL客户端需要重新初始化")

			// 转换数据源配置格式
			convertedDataSources := make(map[string]struct{ DSN string })
			for name, dsConfig := range cfg.Mysql.DataSources {
				convertedDataSources[name] = struct{ DSN string }{DSN: dsConfig.DSN}
			}

			// 重新初始化MySQL客户端
			newDS, err := mysqlclient.NewMultiMysqlClient(
				cfg.Mysql.DSN,
				convertedDataSources,
			)
			if err != nil {
				log.Printf("重新初始化MySQL客户端失败: %v", err)
				continue
			}

			// 更新全局变量
			config.MySQLClient = newDS

			// 关闭旧的连接
			currentDS.Close()

			// 更新当前连接引用
			currentDS = newDS

			log.Println("MySQL客户端重新初始化完成")
		}
	}
}
