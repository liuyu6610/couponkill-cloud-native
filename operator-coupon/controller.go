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

type PodMonitorController struct {
	clientset kubernetes.Interface
	podLister listers.PodLister
	informer  informersv1.PodInformer
}

func NewPodMonitorController(clientset kubernetes.Interface, podInformer informersv1.PodInformer) *PodMonitorController {
	c := &PodMonitorController{
		clientset: clientset,
		podLister: podInformer.Lister(),
		informer:  podInformer,
	}
	// add event handler
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

func (c *PodMonitorController) Run(threadiness int, stopCh <-chan struct{}) {
	log.Println("Starting PodMonitorController")
	<-stopCh
	log.Println("Stopping PodMonitorController")
}

func (c *PodMonitorController) handlePod(pod *v1.Pod) {
	// Only act on pods with label operator/restart-on-failure=true
	labels := pod.GetLabels()
	if labels == nil {
		return
	}
	val, ok := labels["operator/restart-on-failure"]
	if !ok || val != "true" {
		return
	}

	// If pod phase is Failed or Unknown, delete it to let deployment/statefulset recreate
	phase := pod.Status.Phase
	if phase == v1.PodFailed || phase == v1.PodUnknown {
		ns := pod.Namespace
		name := pod.Name
		// add a small delay to avoid flapping
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
