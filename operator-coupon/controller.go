package main

import (
	"context"
	"log"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	informersv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// PodMonitorController 是一个控制器，用于监控 Pod 的状态并根据标签自动删除失败的 Pod。
// 它使用 Kubernetes Informer 机制监听 Pod 的变化事件。
type PodMonitorController struct {
	clientset kubernetes.Interface    // Kubernetes 客户端接口
	podLister listers.PodLister       // Pod 列表器，用于获取 Pod 信息
	informer  informersv1.PodInformer // Pod Informer，用于监听 Pod 事件
}

// NewPodMonitorController 创建一个新的 PodMonitorController 实例。
// 参数：
//   - clientset: Kubernetes 客户端接口，用于与 Kubernetes API 交互
//   - podInformer: Pod Informer，用于监听 Pod 的添加和更新事件
//
// 返回值：
//   - *PodMonitorController: 新创建的控制器实例
func NewPodMonitorController(clientset kubernetes.Interface, podInformer informersv1.PodInformer) *PodMonitorController {
	c := &PodMonitorController{
		clientset: clientset,
		podLister: podInformer.Lister(),
		informer:  podInformer,
	}
	// 添加事件处理器，处理 Pod 的更新和添加事件
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*v1.Pod)
			go c.handlePod(newPod)
		},
		AddFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			go c.handlePod(pod)
		},
	})
	return c
}

// Run 启动控制器的运行逻辑。
// 参数：
//   - threadiness: 控制器并发运行的协程数（当前未使用）
//   - stopCh: 用于通知控制器停止运行的通道
func (c *PodMonitorController) Run(threadiness int, stopCh <-chan struct{}) {
	log.Println("Starting PodMonitorController")
	<-stopCh
	log.Println("Stopping PodMonitorController")
}

// handlePod 处理单个 Pod 的逻辑，检查其标签和状态，并在满足条件时删除 Pod。
// 参数：
//   - pod: 需要处理的 Pod 对象
func (c *PodMonitorController) handlePod(pod *v1.Pod) {
	// 只处理带有 operator/restart-on-failure=true 标签的 Pod
	labels := pod.GetLabels()
	if labels == nil {
		return
	}
	val, ok := labels["operator/restart-on-failure"]
	if !ok || val != "true" {
		return
	}

	// 如果 Pod 处于 Failed 或 Unknown 状态，则删除该 Pod，以便 Deployment/StatefulSet 重新创建
	phase := pod.Status.Phase
	if phase == v1.PodFailed || phase == v1.PodUnknown {
		ns := pod.Namespace
		name := pod.Name
		// 添加短暂延迟以避免频繁抖动
		time.Sleep(2 * time.Second)
		log.Printf("Deleting failed/unknown pod %s/%s as requested by operator label\n", ns, name)
		propagation := metav1.DeletePropagationBackground
		err := c.clientset.CoreV1().Pods(ns).Delete(context.TODO(), name, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil {
			log.Printf("Failed to delete pod %s/%s: %v\n", ns, name, err)
		} else {
			log.Printf("Pod %s/%s deleted by operator\n", ns, name)
		}
	}
}
