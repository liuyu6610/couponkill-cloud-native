// cmd/server/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
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
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 2. 从Nacos获取服务协同配置
	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Nacos.NamespaceId,
		ServerAddr:          cfg.Nacos.ServerAddr,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
	}

	configClient, err := clients.CreateConfigClient(map[string]interface{}{
		"clientConfig": clientConfig,
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

	log.Printf("配置加载成功: 端口=%d, Redis地址=%s, MySQL=%s",
		cfg.Server.Port, cfg.Redis.Addr, cfg.Mysql.DSN)

	// 3. 初始化客户端
	mysqlCli := mysqlclient.NewMysqlClient(cfg.Mysql.DSN)
	defer mysqlCli.Close()

	redisCli := redisclient.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer redisCli.Close()

	// 4. 初始化依赖 - 使用从Nacos获取的Java服务地址
	redisRepo := repository.NewRedisRepository(redisCli, cfg.Seckill.Redis.StockKeyPrefix, "user:received:")
	mysqlRepo := repository.NewMysqlRepository(mysqlCli)
	couponService := service.NewCouponService(cfg.Collaboration.JavaServiceUrl) // 从配置获取Java服务地址
	seckillService := service.NewSeckillService(mysqlRepo, redisRepo, couponService)
	seckillHandler := handler.NewSeckillHandler(seckillService, cfg.Seckill.ValidDays)

	// 5. 路由设置
	r := gin.Default()
	r.POST("/seckill", seckillHandler.HandleSeckill)

	// 添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"port":    cfg.Server.Port,
			"message": "Go服务运行正常",
			"time":    time.Now().Format("2006-01-02 15:04:05"),
		})
	})

	// 6. 启动服务
	log.Printf("Go服务启动成功，端口: %d", cfg.Server.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Server.Port), r))
}
