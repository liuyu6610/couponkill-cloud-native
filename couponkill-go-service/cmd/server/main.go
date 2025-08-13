// cmd/server/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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

	log.Printf("配置加载成功: 端口=%d, Redis地址=%s, MySQL=%s",
		cfg.Server.Port, cfg.Redis.Addr, cfg.Mysql.DSN)

	// 2. 初始化客户端
	mysqlCli := mysqlclient.NewMysqlClient(cfg.Mysql.DSN)
	defer mysqlCli.Close()

	redisCli := redisclient.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer redisCli.Close()

	// 3. 初始化依赖
	redisRepo := repository.NewRedisRepository(redisCli, cfg.Seckill.Redis.StockKeyPrefix, "user:received:")
	mysqlRepo := repository.NewMysqlRepository(mysqlCli)
	couponService := service.NewCouponService("http://couponkill-coupon-service:8082") // 使用正确的服务名
	seckillService := service.NewSeckillService(mysqlRepo, redisRepo, couponService)
	seckillHandler := handler.NewSeckillHandler(seckillService, cfg.Seckill.ValidDays)

	// 4. 路由设置
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

	// 5. 启动服务
	log.Printf("Go服务启动成功，端口: %d", cfg.Server.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Server.Port), r))
}
