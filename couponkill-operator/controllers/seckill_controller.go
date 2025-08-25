// controllers/seckill_controller.go
package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opsv1 "couponkill-operator/api/v1"
)

// SeckillReconciler reconciles a Seckill object
type SeckillReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ops.couponkill.io,resources=seckills,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ops.couponkill.io,resources=seckills/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ops.couponkill.io,resources=seckills/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SeckillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Seckill instance
	seckill := &opsv1.Seckill{}
	err := r.Get(ctx, req.NamespacedName, seckill)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Seckill resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Seckill")
		return ctrl.Result{}, err
	}

	// Reconcile Go Service
	if seckill.Spec.Services.GoService.Enabled {
		if err := r.reconcileService(ctx, seckill, "go", seckill.Spec.Services.GoService); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile Coupon Service
	if seckill.Spec.Services.CouponService.Enabled {
		if err := r.reconcileService(ctx, seckill, "coupon", seckill.Spec.Services.CouponService); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile Order Service
	if seckill.Spec.Services.OrderService.Enabled {
		if err := r.reconcileService(ctx, seckill, "order", seckill.Spec.Services.OrderService); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile User Service
	if seckill.Spec.Services.UserService.Enabled {
		if err := r.reconcileService(ctx, seckill, "user", seckill.Spec.Services.UserService); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile Gateway Service
	if seckill.Spec.Services.GatewayService.Enabled {
		if err := r.reconcileService(ctx, seckill, "gateway", seckill.Spec.Services.GatewayService); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile HPA if enabled
	if seckill.Spec.Scaling.HPA.Enabled {
		if err := r.reconcileHPA(ctx, seckill); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status
	if err := r.updateStatus(ctx, seckill); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileService creates or updates a service and its deployment
func (r *SeckillReconciler) reconcileService(ctx context.Context, seckill *opsv1.Seckill, serviceName string, serviceConfig opsv1.ServiceConfig) error {
	log := log.FromContext(ctx)

	// Define a new Deployment object
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", seckill.Name, serviceName),
			Namespace: seckill.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: serviceConfig.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": serviceName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": serviceName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  serviceName,
						Image: serviceConfig.Image,
						Ports: r.convertPorts(serviceConfig.Ports),
						Resources: corev1.ResourceRequirements{
							Requests: r.convertResourceList(serviceConfig.Resources.Requests),
							Limits:   r.convertResourceList(serviceConfig.Resources.Limits),
						},
						Env: r.convertEnvVars(serviceConfig.Env),
					}},
				},
			},
		},
	}

	// Set Seckill instance as the owner and controller
	if err := ctrl.SetControllerReference(seckill, deployment, r.Scheme); err != nil {
		return err
	}

	// Check if the Deployment already exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Define a new Service object
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-svc", seckill.Name, serviceName),
			Namespace: seckill.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": serviceName,
			},
			Ports: r.convertServicePorts(serviceConfig.Ports),
			Type:  corev1.ServiceTypeClusterIP,
		},
	}

	// Set Seckill instance as the owner and controller
	if err := ctrl.SetControllerReference(seckill, service, r.Scheme); err != nil {
		return err
	}

	// Check if the Service already exists
	foundSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundSvc)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// reconcileHPA creates or updates HorizontalPodAutoscaler
func (r *SeckillReconciler) reconcileHPA(ctx context.Context, seckill *opsv1.Seckill) error {
	log := log.FromContext(ctx)

	// Create HPA for each enabled service
	services := map[string]opsv1.ServiceConfig{
		"go":      seckill.Spec.Services.GoService,
		"coupon":  seckill.Spec.Services.CouponService,
		"order":   seckill.Spec.Services.OrderService,
		"user":    seckill.Spec.Services.UserService,
		"gateway": seckill.Spec.Services.GatewayService,
	}

	for serviceName, serviceConfig := range services {
		if !serviceConfig.Enabled {
			continue
		}

		// Create a pointer to the minReplicas value
		var minReplicas *int32
		if seckill.Spec.Scaling.HPA.MinReplicas != nil {
			minReplicas = seckill.Spec.Scaling.HPA.MinReplicas
		}

		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-hpa", seckill.Name, serviceName),
				Namespace: seckill.Namespace,
			},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind:       "Deployment",
					Name:       fmt.Sprintf("%s-%s", seckill.Name, serviceName),
					APIVersion: "apps/v1",
				},
				MinReplicas: minReplicas,
				MaxReplicas: seckill.Spec.Scaling.HPA.MaxReplicas,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: seckill.Spec.Scaling.HPA.TargetCPUUtilizationPercentage,
							},
						},
					},
				},
			},
		}

		// Set Seckill instance as the owner and controller
		if err := ctrl.SetControllerReference(seckill, hpa, r.Scheme); err != nil {
			return err
		}

		// Check if the HPA already exists
		found := &autoscalingv2.HorizontalPodAutoscaler{}
		err := r.Get(ctx, types.NamespacedName{Name: hpa.Name, Namespace: hpa.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating a new HPA", "HPA.Namespace", hpa.Namespace, "HPA.Name", hpa.Name)
			err = r.Create(ctx, hpa)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return nil
}

// updateStatus updates the Seckill status
func (r *SeckillReconciler) updateStatus(ctx context.Context, seckill *opsv1.Seckill) error {
	// TODO: Implement status update logic
	// This would typically involve checking the status of deployments and services
	// and updating the Seckill status accordingly
	return nil
}

// convertPorts converts ServicePort to ContainerPort
func (r *SeckillReconciler) convertPorts(ports []opsv1.ServicePort) []corev1.ContainerPort {
	containerPorts := make([]corev1.ContainerPort, len(ports))
	for i, port := range ports {
		containerPorts[i] = corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.Port,
			Protocol:      corev1.Protocol(port.Protocol),
		}
	}
	return containerPorts
}

// convertServicePorts converts ServicePort to ServicePort
func (r *SeckillReconciler) convertServicePorts(ports []opsv1.ServicePort) []corev1.ServicePort {
	servicePorts := make([]corev1.ServicePort, len(ports))
	for i, port := range ports {
		servicePorts[i] = corev1.ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: intstr.FromInt(int(port.TargetPort)),
			Protocol:   corev1.Protocol(port.Protocol),
		}
	}
	return servicePorts
}

// convertResourceList converts ResourceList to ResourceList
func (r *SeckillReconciler) convertResourceList(resources opsv1.ResourceList) corev1.ResourceList {
	resourceList := make(corev1.ResourceList)
	for key, value := range resources {
		resourceList[corev1.ResourceName(key)] = resource.MustParse(value)
	}
	return resourceList
}

// convertEnvVars converts EnvVar to EnvVar
func (r *SeckillReconciler) convertEnvVars(envVars []opsv1.EnvVar) []corev1.EnvVar {
	coreEnvVars := make([]corev1.EnvVar, len(envVars))
	for i, envVar := range envVars {
		coreEnvVars[i] = corev1.EnvVar{
			Name:  envVar.Name,
			Value: envVar.Value,
		}
		// Handle ValueFrom if present
		if envVar.ValueFrom != nil {
			coreEnvVars[i].ValueFrom = &corev1.EnvVarSource{}
			if envVar.ValueFrom.ConfigMapKeyRef != nil {
				coreEnvVars[i].ValueFrom.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: envVar.ValueFrom.ConfigMapKeyRef.Name,
					},
					Key: envVar.ValueFrom.ConfigMapKeyRef.Key,
				}
			}
			if envVar.ValueFrom.SecretKeyRef != nil {
				coreEnvVars[i].ValueFrom.SecretKeyRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: envVar.ValueFrom.SecretKeyRef.Name,
					},
					Key: envVar.ValueFrom.SecretKeyRef.Key,
				}
			}
		}
	}
	return coreEnvVars
}

// SetupWithManager sets up the controller with the Manager.
func (r *SeckillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opsv1.Seckill{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}
