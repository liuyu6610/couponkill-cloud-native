package controllers

import (
	"context"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:object:root=true
type SeckillScalePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SeckillScalePolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
type SeckillScalePolicySpec struct {
	GoServiceName string       `json:"goServiceName"`
	GoMinReplicas int32        `json:"goMinReplicas"`
	GoMaxReplicas int32        `json:"goMaxReplicas"`
	OtherServices []OtherScale `json:"otherServices,omitempty"`
	CpuTarget     int32        `json:"cpuTarget"`
}
type OtherScale struct {
	Name        string `json:"name"`
	MinReplicas int32  `json:"minReplicas"`
	MaxReplicas int32  `json:"maxReplicas"`
}

// Reconcile: ensure HPAs exist for go-service and other services
func Reconcile(ctx context.Context, c client.Client, ns string, policyName string) error {
	logger := log.FromContext(ctx)
	var p SeckillScalePolicy
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: policyName}, &p); err != nil {
		return err
	}
	// Helper to create/update HPA
	upsertHPA := func(name string, min, max, cpuTarget int32) error {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, c, hpa, func() error {
			hpa.Spec = autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min,
				MaxReplicas: max,
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment", Name: name, APIVersion: "apps/v1",
				},
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &cpuTarget,
						},
					},
				}},
			}
			return nil
		})
		return err
	}
	if err := upsertHPA(p.Spec.GoServiceName, p.Spec.GoMinReplicas, p.Spec.GoMaxReplicas, p.Spec.CpuTarget); err != nil {
		logger.Error(err, "upsert HPA for go-service failed")
		return err
	}
	for _, s := range p.Spec.OtherServices {
		if err := upsertHPA(s.Name, s.MinReplicas, s.MaxReplicas, p.Spec.CpuTarget); err != nil {
			logger.Error(err, "upsert HPA for service failed", "svc", s.Name)
			return err
		}
	}
	return nil
}
