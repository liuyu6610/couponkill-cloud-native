// operator-coupon/main.go
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// main 是程序的入口函数，负责初始化 Kubernetes 客户端、启动控制器和监听中断信号以实现优雅关闭。
func main() {
	// 解析命令行参数
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file (optional)")
	masterURL := flag.String("master", "", "master url (optional)")
	flag.Parse()

	// 构建Kubernetes配置
	var cfg *rest.Config
	var err error
	if *kubeconfig != "" {
		// 从外部kubeconfig文件构建配置（适用于本地开发）
		cfg, err = clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	} else {
		// 从集群内部构建配置（适用于集群内运行）
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// 创建Kubernetes客户端
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}

	// 创建共享Informer工厂，用于监听Pod变化
	// 每30秒重新同步一次
	factory := informers.NewSharedInformerFactory(clientset, 30*time.Second)

	// 创建并启动控制器
	controller := NewPodMonitorController(clientset, factory.Core().V1().Pods())
	stopCh := make(chan struct{})
	defer close(stopCh)

	// 启动控制器，使用2个工作线程
	go controller.Run(2, stopCh)
	// 启动Informer
	factory.Start(stopCh)

	// 等待中断信号，优雅退出
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	log.Println("Shutting down operator")
}
