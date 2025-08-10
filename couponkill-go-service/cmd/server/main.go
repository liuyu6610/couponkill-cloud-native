// cmd/server/main.go
package main

import (
	"fmt"
	"log"
	"net/http"

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

	// 2. 初始化客户端
	mysqlCli := mysqlclient.NewMysqlClient(cfg.Mysql.DSN)
	defer mysqlCli.Close()
	redisCli := redisclient.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer redisCli.Close()

	// 3. 初始化依赖
	redisRepo := repository.NewRedisRepository(redisCli, cfg.Seckill.RedisStockPrefix)
	mysqlRepo := repository.NewMysqlRepository(mysqlCli)
	seckillService := service.NewSeckillService(mysqlRepo, redisRepo)
	seckillHandler := handler.NewSeckillHandler(seckillService, cfg.Seckill.ValidDays)

	// 4. 路由设置
	r := gin.Default()
	r.POST("/seckill", seckillHandler.HandleSeckill)

	// 5. 启动服务
	log.Printf("Go服务启动成功，端口: %d", cfg.Server.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.Server.Port), r))
}
