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
// SeckillScalePolicy 是一个用于定义秒杀场景下服务自动扩缩容策略的自定义资源。
// 它描述了 Go 服务和其他服务的扩缩容参数以及 CPU 使用率目标。
type SeckillScalePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SeckillScalePolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
// SeckillScalePolicySpec 定义了 SeckillScalePolicy 的规格说明。
// 包括 Go 服务的名称、最小/最大副本数、其他服务的扩缩容配置以及 CPU 使用率目标。
type SeckillScalePolicySpec struct {
	// GoServiceName 是需要进行自动扩缩容的 Go 服务 Deployment 名称
	GoServiceName string `json:"goServiceName"`

	// GoMinReplicas 是 Go 服务允许的最小副本数
	GoMinReplicas int32 `json:"goMinReplicas"`

	// GoMaxReplicas 是 Go 服务允许的最大副本数
	GoMaxReplicas int32 `json:"goMaxReplicas"`

	// OtherServices 是其他需要进行自动扩缩容的服务列表
	OtherServices []OtherScale `json:"otherServices,omitempty"`

	// CpuTarget 是所有服务使用的 CPU 平均使用率目标值（百分比）
	CpuTarget int32 `json:"cpuTarget"`
}

// OtherScale 定义了除 Go 服务外的其他服务的扩缩容参数。
type OtherScale struct {
	// Name 是该服务对应的 Deployment 名称
	Name string `json:"name"`

	// MinReplicas 是该服务允许的最小副本数
	MinReplicas int32 `json:"minReplicas"`

	// MaxReplicas 是该服务允许的最大副本数
	MaxReplicas int32 `json:"maxReplicas"`
}

// Reconcile: ensure HPAs exist for go-service and other services

// Reconcile 根据 SeckillScalePolicy 策略确保为 Go 服务及其他服务创建或更新对应的 HorizontalPodAutoscaler (HPA) 资源。
// 参数：
//   - ctx: 上下文信息，用于控制请求的生命周期和传递元数据
//   - c: controller-runtime 提供的 client，用于与 Kubernetes API 交互
//   - ns: SeckillScalePolicy 所在的命名空间
//   - policyName: SeckillScalePolicy 的名称
//
// 返回值：
//   - error: 如果获取策略或创建/更新 HPA 失败，则返回错误
func Reconcile(ctx context.Context, c client.Client, ns string, policyName string) error {
	logger := log.FromContext(ctx)

	// 获取指定的 SeckillScalePolicy 策略对象
	var p SeckillScalePolicy
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: policyName}, &p); err != nil {
		return err
	}

	// 定义一个辅助函数，用于创建或更新 HPA 资源
	upsertHPA := func(name string, min, max, cpuTarget int32) error {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, c, hpa, func() error {
			// 设置 HPA 的扩缩容范围和目标引用
			hpa.Spec = autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min,
				MaxReplicas: max,
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind:       "Deployment",
					Name:       name,
					APIVersion: "apps/v1",
				},
				// 配置基于 CPU 使用率的扩缩容指标
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

	// 为 Go 服务创建或更新 HPA
	if err := upsertHPA(p.Spec.GoServiceName, p.Spec.GoMinReplicas, p.Spec.GoMaxReplicas, p.Spec.CpuTarget); err != nil {
		logger.Error(err, "upsert HPA for go-service failed")
		return err
	}

	// 为其他服务逐个创建或更新 HPA
	for _, s := range p.Spec.OtherServices {
		if err := upsertHPA(s.Name, s.MinReplicas, s.MaxReplicas, p.Spec.CpuTarget); err != nil {
			logger.Error(err, "upsert HPA for service failed", "svc", s.Name)
			return err
		}
	}

	return nil
}
