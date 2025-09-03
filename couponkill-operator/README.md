# CouponKill Operator

CouponKill Operator 是一个基于 Kubernetes Operator 模式的自定义控制器，用于管理和自动化部署 CouponKill 秒杀系统。它通过自定义资源定义 (CRD) 提供了一种声明式的方式来管理整个秒杀系统的部署、配置和运维。

## 功能特性

### 1. 声明式部署管理
- 通过 Seckill 自定义资源 (CR) 声明式地定义和管理整个秒杀系统
- 自动创建和管理所有微服务的 Deployment 和 Service
- 支持独立启用或禁用特定服务

### 2. 自动扩缩容
- 支持 HorizontalPodAutoscaler (HPA) 自动扩缩容
- 集成 KEDA 实现基于 Kafka 消费者延迟的自动扩缩容
- 可配置最小和最大副本数以及目标 CPU 使用率

### 3. 监控集成
- 集成 Prometheus 和 Grafana 监控
- 提供系统状态监控和指标收集
- 支持自定义监控配置

### 4. 配置管理
- 统一管理所有服务的配置参数
- 支持环境变量、ConfigMap 和 Secret 配置
- 提供灵活的资源配置选项

## 架构设计

### 核心组件

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CouponKill Operator                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  Controller Manager                                                         │
│  ├── Seckill Reconciler                                                     │
│  │   ├── Deployment Controller                                              │
│  │   ├── Service Controller                                                 │
│  │   ├── HPA Controller                                                     │
│  │   └── Status Controller                                                  │
│  └── Custom Resource Definitions (CRDs)                                     │
│      └── Seckill CRD                                                        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 工作原理

CouponKill Operator 遵循 Kubernetes Operator 模式，通过监听 Seckill 自定义资源的变化来协调集群状态：

1. **资源监听**: Operator 监听 Seckill 资源的创建、更新和删除事件
2. **状态协调**: 当检测到资源变化时，触发 Reconcile 循环
3. **资源创建**: 根据 Seckill 资源的配置创建相应的 Kubernetes 资源 (Deployment、Service、HPA 等)
4. **状态更新**: 持续监控所管理资源的状态并更新 Seckill 资源状态
5. **垃圾回收**: 当 Seckill 资源被删除时，自动清理相关资源

### Reconcile 流程

```
┌────────────────────┐
│   Seckill CR 变化   │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│   触发 Reconcile   │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│  检查 Go 服务配置   │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│  检查优惠券服务配置 │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│   检查订单服务配置  │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│   检查用户服务配置  │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│  检查网关服务配置   │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│   检查自动扩缩容    │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│    更新状态信息     │
└─────────┬──────────┘
          │
          ▼
┌────────────────────┐
│      完成循环       │
└────────────────────┘
```

## 安装部署

### 前提条件
- Kubernetes 集群 1.16+
- kubectl 命令行工具
- Helm 3.0+ (可选)

### 部署步骤

#### 1. 安装 CRD
```bash
kubectl apply -f config/crd/bases/
```

#### 2. 安装 RBAC 权限
```bash
kubectl apply -f config/rbac/
```

#### 3. 部署 Operator
```bash
kubectl apply -f config/manager/
```

#### 4. 部署 Seckill 实例
```bash
kubectl apply -f sample/seckill.yaml
```

或者使用 Makefile:
```bash
make deploy
```

## 使用说明

### 创建 Seckill 资源

创建一个 Seckill 资源来部署整个秒杀系统：

```yaml
apiVersion: ops.couponkill.io/v1
kind: Seckill
metadata:
  name: couponkill-sample
  namespace: couponkill
spec:
  services:
    goService:
      enabled: true
      image: "seckill-go:latest"
      replicas: 2
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"
      ports:
        - name: http
          port: 8080
          targetPort: 8080
          protocol: TCP
      env:
        - name: NACOS_SERVER_ADDR
          value: "nacos:8848"
    couponService:
      enabled: true
      image: "coupon:latest"
      replicas: 2
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"
      ports:
        - name: http
          port: 8080
          targetPort: 8080
          protocol: TCP
    orderService:
      enabled: true
      image: "order:latest"
      replicas: 2
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"
      ports:
        - name: http
          port: 8080
          targetPort: 8080
          protocol: TCP
    userService:
      enabled: true
      image: "user:latest"
      replicas: 1
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"
      ports:
        - name: http
          port: 8080
          targetPort: 8080
          protocol: TCP
    gatewayService:
      enabled: true
      image: "gateway:latest"
      replicas: 1
      resources:
        requests:
          memory: "512Mi"
          cpu: "250m"
        limits:
          memory: "1Gi"
          cpu: "500m"
      ports:
        - name: http
          port: 8080
          targetPort: 8080
          protocol: TCP
  scaling:
    hpa:
      enabled: true
      minReplicas: 1
      maxReplicas: 10
      targetCPUUtilizationPercentage: 80
    keda:
      enabled: true
      kafkaScaler:
        bootstrapServers: "broker:9092"
        consumerGroup: "seckill-go-group"
        topic: "seckill_order_create"
        lagThreshold: "5"
  monitoring:
    enabled: true
    prometheusEnabled: true
    grafanaEnabled: true
```

### 配置说明

#### Services 配置
每个服务都可以独立配置以下参数：
- `enabled`: 是否启用该服务
- `image`: 服务镜像
- `replicas`: 副本数
- `resources`: 资源请求和限制
- `ports`: 服务端口配置
- `env`: 环境变量配置

#### Scaling 配置
支持两种自动扩缩容方式：
1. **HPA**: 基于 CPU 使用率的自动扩缩容
2. **KEDA**: 基于 Kafka 消费者延迟的自动扩缩容

#### Monitoring 配置
- `prometheusEnabled`: 是否启用 Prometheus 监控
- `grafanaEnabled`: 是否启用 Grafana 监控

## 开发指南

### 项目结构
```
couponkill-operator/
├── api/
│   └── v1/
│       ├── seckill_types.go     # Seckill CRD 定义
│       └── groupversion_info.go # API 组版本信息
├── controllers/
│   └── seckill_controller.go    # Seckill 控制器实现
├── config/
│   ├── crd/                     # CRD 配置
│   ├── rbac/                    # RBAC 权限配置
│   └── manager/                 # Operator 管理器配置
├── sample/
│   └── seckill.yaml            # Seckill 示例配置
├── main.go                     # Operator 入口点
└── Dockerfile                  # Operator 镜像构建文件
```

### 构建和部署

#### 本地运行
```bash
# 生成代码和清单
make generate
make manifests

# 运行 Operator
make run
```

#### 构建镜像
```bash
# 构建 Operator 镜像
docker build -t couponkill-operator:latest .

# 推送镜像到仓库
docker push couponkill-operator:latest
```

#### 部署到集群
```bash
# 安装 CRD
make install

# 部署 Operator
make deploy

# 卸载 Operator
make undeploy
```

### 控制器实现

Seckill 控制器主要实现了以下功能：

1. **服务协调**: 根据 Seckill 资源配置创建和管理各个服务的 Deployment 和 Service
2. **自动扩缩容**: 创建和管理 HPA 资源实现自动扩缩容
3. **状态管理**: 更新 Seckill 资源状态以反映实际部署情况

核心代码逻辑在 `controllers/seckill_controller.go` 文件中：

```go
// Reconcile 主要协调逻辑
func (r *SeckillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 获取 Seckill 资源
    seckill := &opsv1.Seckill{}
    err := r.Get(ctx, req.NamespacedName, seckill)
    
    // 为每个启用的服务创建 Deployment 和 Service
    if seckill.Spec.Services.GoService.Enabled {
        if err := r.reconcileService(ctx, seckill, "go", seckill.Spec.Services.GoService); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    // ... 其他服务
    
    // 协调自动扩缩容
    if seckill.Spec.Scaling.HPA.Enabled {
        if err := r.reconcileHPA(ctx, seckill); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    // 更新状态
    if err := r.updateStatus(ctx, seckill); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

## 故障排除

### 常见问题

1. **Operator 无法启动**
   - 检查 RBAC 权限配置
   - 确认 CRD 已正确安装
   - 查看 Operator 日志: `kubectl logs -n couponkill deployment/couponkill-operator`

2. **服务无法部署**
   - 检查 Seckill 资源配置是否正确
   - 确认镜像名称和标签正确
   - 查看相关 Deployment 和 Service 的状态

3. **自动扩缩容不工作**
   - 确认 HPA 或 KEDA 配置正确
   - 检查 Metrics Server 是否正常运行
   - 验证资源请求和限制是否已设置

### 查看状态

```bash
# 查看 Seckill 资源
kubectl get seckill -n couponkill

# 查看 Seckill 资源详细信息
kubectl describe seckill couponkill-sample -n couponkill

# 查看 Operator 日志
kubectl logs -n couponkill deployment/couponkill-operator -f

# 查看服务状态
kubectl get deployments,services -n couponkill
```

## 最佳实践

1. **资源管理**: 为每个服务合理配置资源请求和限制，避免资源浪费或不足
2. **自动扩缩容**: 根据实际负载情况配置自动扩缩容参数
3. **监控告警**: 启用监控功能并配置合适的告警规则
4. **版本管理**: 使用版本化的镜像标签，避免使用 latest 标签
5. **安全配置**: 为敏感配置使用 Secret 而不是明文环境变量
6. **备份恢复**: 定期备份 Seckill 资源配置，便于灾难恢复