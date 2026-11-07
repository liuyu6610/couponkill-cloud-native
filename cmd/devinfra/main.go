// Command devinfra 在本机启动轻量的开发用基础设施替身：miniredis（Redis 协议，含 Lua）与 kfake（Kafka 协议，进程内）。
//
// 它只用于没有 Docker 的本机联调与端到端冒烟，数据全在内存里、进程退出即丢，绝不能用于生产。
// PostgreSQL 不在其中：请使用真实的 PostgreSQL（本仓库的测试与冒烟都假定如此）。
//
//	go run ./cmd/devinfra -redis 127.0.0.1:6379 -kafka-port 9092
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alicebob/miniredis/v2"
	"github.com/twmb/franz-go/pkg/kfake"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
)

func main() {
	if err := run(); err != nil {
		slog.Error("devinfra exited with an error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	redisAddr := flag.String("redis", "127.0.0.1:6379", "miniredis 监听地址")
	kafkaPort := flag.Int("kafka-port", 9092, "kfake 单 broker 监听端口")
	partitions := flag.Int("partitions", 4, "秒杀建单主题的分区数")
	flag.Parse()

	rds := miniredis.NewMiniRedis()
	if err := rds.StartAddr(*redisAddr); err != nil {
		return fmt.Errorf("start miniredis on %s: %w", *redisAddr, err)
	}
	defer rds.Close()

	// 与 Helm/compose 的 topic-init 保持一致：建单主题 + 死信主题。
	cluster, err := kfake.NewCluster(
		kfake.NumBrokers(1),
		kfake.Ports(*kafkaPort),
		kfake.SeedTopics(int32(*partitions), events.TopicSeckillOrderCreate),
		kfake.SeedTopics(1, events.TopicSeckillOrderCreateDLT),
	)
	if err != nil {
		return fmt.Errorf("start kfake on port %d: %w", *kafkaPort, err)
	}
	defer cluster.Close()

	slog.Info("devinfra ready (in-memory, dev only)",
		"redis", rds.Addr(), "kafka", cluster.ListenAddrs(),
		"topics", []string{events.TopicSeckillOrderCreate, events.TopicSeckillOrderCreateDLT})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	slog.Info("devinfra stopping")
	return nil
}
