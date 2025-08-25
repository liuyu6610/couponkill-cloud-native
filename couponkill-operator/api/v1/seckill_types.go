// api/v1/seckill_types.go
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SeckillSpec defines the desired state of Seckill
type SeckillSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Services defines the configuration for all microservices
	Services ServicesSpec `json:"services,omitempty"`

	// Scaling defines the autoscaling configuration
	Scaling ScalingSpec `json:"scaling,omitempty"`

	// Monitoring defines the monitoring configuration
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`
}

// ServicesSpec defines the configuration for all microservices
type ServicesSpec struct {
	// GoService defines the configuration for the Go service
	GoService ServiceConfig `json:"goService,omitempty"`

	// CouponService defines the configuration for the coupon service
	CouponService ServiceConfig `json:"couponService,omitempty"`

	// OrderService defines the configuration for the order service
	OrderService ServiceConfig `json:"orderService,omitempty"`

	// UserService defines the configuration for the user service
	UserService ServiceConfig `json:"userService,omitempty"`

	// GatewayService defines the configuration for the gateway service
	GatewayService ServiceConfig `json:"gatewayService,omitempty"`
}

// ServiceConfig defines the configuration for a single service
type ServiceConfig struct {
	// Enabled indicates whether the service is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Image defines the container image for the service
	Image string `json:"image,omitempty"`

	// Replicas is the desired number of replicas
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources defines the resource requirements for the service
	Resources ResourceRequirements `json:"resources,omitempty"`

	// Ports defines the ports exposed by the service
	Ports []ServicePort `json:"ports,omitempty"`

	// Environment variables for the service
	Env []EnvVar `json:"env,omitempty"`
}

// ResourceRequirements describes the resource requirements for a service
type ResourceRequirements struct {
	// Requests describes the minimum amount of compute resources required
	Requests ResourceList `json:"requests,omitempty"`

	// Limits describes the maximum amount of compute resources allowed
	Limits ResourceList `json:"limits,omitempty"`
}

// ResourceList is a set of (resource name, quantity) pairs
type ResourceList map[string]string

// ServicePort describes the port exposed by a service
type ServicePort struct {
	// Name for the port
	Name string `json:"name,omitempty"`

	// Port number
	Port int32 `json:"port,omitempty"`

	// TargetPort number
	TargetPort int32 `json:"targetPort,omitempty"`

	// Protocol for the port
	Protocol string `json:"protocol,omitempty"`
}

// EnvVar represents an environment variable present in a Container
type EnvVar struct {
	// Name of the environment variable
	Name string `json:"name"`

	// Value of the environment variable
	Value string `json:"value,omitempty"`

	// ValueFrom defines a source for the environment variable's value
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

// EnvVarSource represents a source for the value of an EnvVar
type EnvVarSource struct {
	// Selects a key of a ConfigMap
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// Selects a key of a Secret
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// ConfigMapKeySelector selects a key from a ConfigMap
type ConfigMapKeySelector struct {
	// The name of the ConfigMap to select from
	Name string `json:"name"`

	// The key to select
	Key string `json:"key"`
}

// SecretKeySelector selects a key from a Secret
type SecretKeySelector struct {
	// The name of the Secret to select from
	Name string `json:"name"`

	// The key to select
	Key string `json:"key"`
}

// ScalingSpec defines the autoscaling configuration
type ScalingSpec struct {
	// HPA defines the HorizontalPodAutoscaler configuration
	HPA HPASpec `json:"hpa,omitempty"`

	// KEDA defines the KEDA configuration
	KEDA KEDASpec `json:"keda,omitempty"`
}

// HPASpec defines the HorizontalPodAutoscaler configuration
type HPASpec struct {
	// Enabled indicates whether HPA is enabled
	Enabled bool `json:"enabled,omitempty"`

	// MinReplicas is the minimum number of replicas
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// TargetCPUUtilizationPercentage is the target CPU utilization percentage
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
}

// KEDASpec defines the KEDA configuration
type KEDASpec struct {
	// Enabled indicates whether KEDA is enabled
	Enabled bool `json:"enabled,omitempty"`

	// KafkaScaler defines the Kafka scaler configuration
	KafkaScaler KafkaScalerSpec `json:"kafkaScaler,omitempty"`
}

// KafkaScalerSpec defines the Kafka scaler configuration
type KafkaScalerSpec struct {
	// BootstrapServers is the Kafka bootstrap servers
	BootstrapServers string `json:"bootstrapServers,omitempty"`

	// ConsumerGroup is the Kafka consumer group
	ConsumerGroup string `json:"consumerGroup,omitempty"`

	// Topic is the Kafka topic
	Topic string `json:"topic,omitempty"`

	// LagThreshold is the lag threshold
	LagThreshold string `json:"lagThreshold,omitempty"`
}

// MonitoringSpec defines the monitoring configuration
type MonitoringSpec struct {
	// Enabled indicates whether monitoring is enabled
	Enabled bool `json:"enabled,omitempty"`

	// PrometheusEnabled indicates whether Prometheus monitoring is enabled
	PrometheusEnabled bool `json:"prometheusEnabled,omitempty"`

	// GrafanaEnabled indicates whether Grafana monitoring is enabled
	GrafanaEnabled bool `json:"grafanaEnabled,omitempty"`
}

// SeckillStatus defines the observed state of Seckill
type SeckillStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of a Seckill's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase is the current lifecycle phase of the Seckill
	Phase string `json:"phase,omitempty"`

	// ServiceStatuses contains the status of each service
	ServiceStatuses map[string]ServiceStatus `json:"serviceStatuses,omitempty"`
}

// ServiceStatus defines the status of a service
type ServiceStatus struct {
	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// TotalReplicas is the total number of replicas
	TotalReplicas int32 `json:"totalReplicas,omitempty"`

	// Phase is the current phase of the service
	Phase string `json:"phase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Seckill is the Schema for the seckills API
type Seckill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SeckillSpec   `json:"spec,omitempty"`
	Status SeckillStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SeckillList contains a list of Seckill
type SeckillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Seckill `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Seckill{}, &SeckillList{})
}
