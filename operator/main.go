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

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file (optional)")
	masterURL := flag.String("master", "", "master url (optional)")
	flag.Parse()

	// Build config
	var cfg *rest.Config
	var err error
	if *kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}

	// Shared informer factory
	factory := informers.NewSharedInformerFactory(clientset, 30*time.Second)

	// start controller
	controller := NewPodMonitorController(clientset, factory.Core().V1().Pods())
	stopCh := make(chan struct{})
	defer close(stopCh)

	go controller.Run(2, stopCh)
	factory.Start(stopCh)

	// wait for termination signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	log.Println("Shutting down operator")
}
