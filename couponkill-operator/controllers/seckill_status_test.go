package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	opsv1 "couponkill-operator/api/v1"
)

func TestUpdateStatus_AllReady(t *testing.T) {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = opsv1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	replicas := int32(2)
	seckill := &opsv1.Seckill{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default", Generation: 1},
		Spec: opsv1.SeckillSpec{
			Services: opsv1.ServicesSpec{
				CouponService: opsv1.ServiceConfig{Enabled: true, Replicas: &replicas},
				OrderService:  opsv1.ServiceConfig{Enabled: true, Replicas: &replicas},
			},
		},
	}
	couponDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-coupon", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{Replicas: 2, ReadyReplicas: 2},
	}
	orderDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-order", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{Replicas: 2, ReadyReplicas: 2},
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(seckill).
		WithObjects(seckill, couponDep, orderDep).Build()
	r := &SeckillReconciler{Client: cl, Scheme: s}

	nn := types.NamespacedName{Name: "demo", Namespace: "default"}
	got := &opsv1.Seckill{}
	if err := cl.Get(context.Background(), nn, got); err != nil {
		t.Fatalf("get seckill: %v", err)
	}
	if err := r.updateStatus(context.Background(), got); err != nil {
		t.Fatalf("updateStatus: %v", err)
	}
	if err := cl.Get(context.Background(), nn, got); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase=%s want Ready", got.Status.Phase)
	}
	if got.Status.ServiceStatuses["coupon"].Phase != "Ready" {
		t.Fatalf("coupon phase=%s", got.Status.ServiceStatuses["coupon"].Phase)
	}
	var readyCond metav1.Condition
	for _, c := range got.Status.Conditions {
		if c.Type == "Ready" {
			readyCond = c
			break
		}
	}
	if readyCond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition=%s reason=%s", readyCond.Status, readyCond.Reason)
	}
}

func TestUpdateStatus_ProgressingWhenMissing(t *testing.T) {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = opsv1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	replicas := int32(1)
	seckill := &opsv1.Seckill{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default", Generation: 1},
		Spec: opsv1.SeckillSpec{
			Services: opsv1.ServicesSpec{
				GatewayService: opsv1.ServiceConfig{Enabled: true, Replicas: &replicas},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(seckill).
		WithObjects(seckill).Build()
	r := &SeckillReconciler{Client: cl, Scheme: s}

	nn := types.NamespacedName{Name: "demo", Namespace: "default"}
	got := &opsv1.Seckill{}
	_ = cl.Get(context.Background(), nn, got)
	if err := r.updateStatus(context.Background(), got); err != nil {
		t.Fatalf("updateStatus: %v", err)
	}
	_ = cl.Get(context.Background(), nn, got)
	if got.Status.Phase != "Progressing" {
		t.Fatalf("phase=%s want Progressing", got.Status.Phase)
	}
	if got.Status.ServiceStatuses["gateway"].Phase != "Pending" {
		t.Fatalf("gateway phase=%s", got.Status.ServiceStatuses["gateway"].Phase)
	}
}
