# CouponKill Istio 配置

CouponKill 系统提供了完整的 Istio 服务网格配置，用于实现高级的流量管理、安全控制和可观察性功能。这些配置位于 [k8s-istio](.) 目录中，作为 Helm Chart 中基础 Istio 功能的补充。

## 目录结构

```
k8s-istio/
├── istio_config.yaml              # 命名空间配置和自动注入设置
├── istio_gateway.yaml             # 网关配置
├── istio_AuthorizationPolicy.yaml # 授权策略
├── istio_DestinationRule.yaml     # 目标规则
├── istio_PeerAuthentication.yaml  # 对等认证
├── istio_ServiceEntry.yaml        # 服务条目
├── istio_Sidecar.yaml             # Sidecar 配置
├── istio_Telemetry.yaml           # 遥测配置
├── istio_VirtualService.yaml      # 虚拟服务
└── istio_opaGateway.yaml          # 网关策略配置
```

## 功能概述

### 1. 流量管理

通过 VirtualService 和 DestinationRule 实现细粒度的流量控制：

- **路由规则**: 精确控制服务间的请求路由
- **负载均衡**: 配置不同的负载均衡算法
- **故障处理**: 设置超时、重试和熔断策略
- **故障注入**: 支持延迟和中止故障注入，用于测试系统弹性

### 2. 安全控制

通过多种安全策略确保服务间通信的安全性：

- **认证**: 配置服务间 mTLS 认证
- **授权**: 细粒度的访问控制策略
- **外部服务访问**: 安全地访问外部服务

### 3. 可观察性

通过遥测配置增强系统的可观察性：

- **指标收集**: 自定义指标和标签
- **访问日志**: 详细的访问日志记录
- **分布式追踪**: 与 Istio 的分布式追踪集成

### 4. 网络策略

通过 Sidecar 和 Gateway 配置优化网络通信：

- **Sidecar**: 优化服务间通信
- **Gateway**: 管理入口和出口流量

## 配置详解

### 命名空间和自动注入 (istio_config.yaml)

配置 couponkill 命名空间启用 Istio sidecar 自动注入：

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: couponkill
  labels:
    istio-injection: enabled
```

### 网关配置 (istio_gateway.yaml)

配置入口网关监听 80 和 443 端口：

```yaml
servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
      - "*"
  - port:
      number: 443
      name: https
      protocol: HTTPS
    tls:
      mode: SIMPLE
      credentialName: couponkill-credential
    hosts:
      - "*"
```

### 授权策略 (istio_AuthorizationPolicy.yaml)

配置多种授权策略：

1. 允许入口网关访问所有服务
2. 允许服务间相互访问
3. 为每个服务设置特定的安全策略

例如，Go 服务的安全策略：

```yaml
rules:
- from:
  - source:
      principals:
      - "cluster.local/ns/couponkill/sa/couponkill-order-service"
      - "cluster.local/ns/istio-system/sa/istio-ingressgateway-service-account"
  when:
  - key: request.headers[x-api-key]
    values: ["seckill-api-key"]
```

### 目标规则 (istio_DestinationRule.yaml)

为每个服务配置流量策略：

```yaml
trafficPolicy:
  connectionPool:
    tcp:
      maxConnections: 100
    http:
      http1MaxPendingRequests: 50
      maxRequestsPerConnection: 10
  outlierDetection:
    consecutiveErrors: 5
    interval: 10s
    baseEjectionTime: 30s
    maxEjectionPercent: 50
  tls:
    mode: ISTIO_MUTUAL
  loadBalancer:
    simple: LEAST_REQUEST
```

### 对等认证 (istio_PeerAuthentication.yaml)

配置服务间 mTLS 认证：

```yaml
mtls:
  mode: STRICT
```

### 服务条目 (istio_ServiceEntry.yaml)

配置对外部服务的访问：

```yaml
hosts:
- "api.alibaba.com"
- "oss.aliyuncs.com"
ports:
- number: 443
  name: https
  protocol: HTTPS
location: MESH_EXTERNAL
resolution: DNS
```

### Sidecar 配置 (istio_Sidecar.yaml)

优化服务间通信，所有服务均使用80端口：

```yaml
egress:
- hosts:
  - "./*"
  - "istio-system/*"
ingress:
- port:
    number: 80
    protocol: HTTP
    name: seckill-go
  defaultEndpoint: 0.0.0.0:80
```

### 遥测配置 (istio_Telemetry.yaml)

自定义遥测配置：

```yaml
accessLogging:
- providers:
  - name: envoy
  filter:
    expression: response.code >= 400
metrics:
- providers:
  - name: prometheus
  overrides:
  - tagOverrides:
      response_code:
        value: "string(response.code)"
```

### 虚拟服务 (istio_VirtualService.yaml)

配置路由规则和故障处理：

```yaml
http:
- match:
  - uri:
      prefix: /seckill
  route:
  - destination:
      host: seckill-go-svc.couponkill.svc.cluster.local
      port:
        number: 80
  retries:
    attempts: 3
    perTryTimeout: 2s
    retryOn: gateway-error,connect-failure,refused-stream
  timeout: 5s
  fault:
    delay:
      percentage:
        value: 0.1
      fixedDelay: 5s
    abort:
      percentage:
        value: 0.1
      httpStatus: 400
```

### 网关策略 (istio_opaGateway.yaml)

配置网关级别的限流和 CORS：

```yaml
cors:
  allow_origin:
  - "*"
  allow_methods:
  - POST
  - GET
  - OPTIONS
  - PUT
  - DELETE
  allow_headers:
  - "*"
  max_age: "86400"
```

## 部署说明

### 前提条件

1. 确保已安装 Istio 控制平面
2. 确保 CouponKill 应用已部署在 couponkill 命名空间中

### 部署步骤

```bash
# 应用所有 Istio 配置
kubectl apply -f k8s-istio/
```

### 验证配置

```bash
# 检查 VirtualService
kubectl get virtualservices -n couponkill

# 检查 DestinationRule
kubectl get destinationrules -n couponkill

# 检查 AuthorizationPolicy
kubectl get authorizationpolicies -n couponkill

# 查看 Istio 配置状态
istioctl proxy-status
```

## 定制化配置

### 修改路由规则

编辑 [istio_VirtualService.yaml](istio_VirtualService.yaml) 文件，根据需要调整路由规则。

### 调整安全策略

编辑 [istio_AuthorizationPolicy.yaml](istio_AuthorizationPolicy.yaml) 文件，修改授权策略。

### 优化流量策略

编辑 [istio_DestinationRule.yaml](istio_DestinationRule.yaml) 文件，调整连接池和熔断器设置。

### 自定义遥测

编辑 [istio_Telemetry.yaml](istio_Telemetry.yaml) 文件，配置自定义指标和日志。

## 故障排除

### 常见问题

1. **Sidecar 注入失败**
   - 检查命名空间是否启用了自动注入
   - 确认 Istio sidecar injector 是否正常运行

2. **路由不生效**
   - 检查 VirtualService 配置是否正确
   - 确认服务主机名是否匹配

3. **安全策略阻止访问**
   - 检查 AuthorizationPolicy 配置
   - 确认服务账户是否正确

4. **外部服务访问失败**
   - 检查 ServiceEntry 配置
   - 确认网络策略是否允许访问

### 调试命令

```bash
# 查看配置状态
istioctl proxy-status

# 查看配置详情
istioctl proxy-config cluster <pod-name>.couponkill

# 查看路由配置
istioctl proxy-config route <pod-name>.couponkill

# 查看监听器配置
istioctl proxy-config listener <pod-name>.couponkill
```

## 最佳实践

1. **渐进式部署**: 先部署基础配置，再逐步添加高级功能
2. **监控和日志**: 启用详细的访问日志和指标收集
3. **安全优先**: 严格控制服务间访问权限
4. **性能调优**: 根据实际负载调整连接池和熔断器参数
5. **定期审查**: 定期审查和优化配置策略