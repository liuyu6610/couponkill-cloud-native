# 跨命名空间监控配置示例

这个示例展示了如何配置Prometheus Operator在`monitoring`命名空间中监控`couponkill`命名空间中的服务。

## 目录结构

```
cross-namespace-monitoring/
├── monitoring-resources.yaml  # ServiceMonitor资源配置
├── rbac.yaml                 # RBAC权限配置
├── values.yaml               # Helm配置示例
└── README.md                 # 说明文档
```

## 配置说明

### 1. ServiceMonitor资源配置

在`monitoring-resources.yaml`中，我们定义了多个ServiceMonitor资源，每个资源负责监控特定的服务：

- `couponkill-go-monitor`: 监控Go服务
- `couponkill-user-monitor`: 监控用户服务
- `couponkill-order-monitor`: 监控订单服务
- `couponkill-coupon-monitor`: 监控优惠券服务
- `couponkill-gateway-monitor`: 监控网关服务

每个ServiceMonitor都包含以下关键配置：

```yaml
spec:
  # 指定要监控哪个命名空间中的服务
  namespaceSelector:
    matchNames:
      - couponkill  # 被监控服务所在的命名空间
  
  # 选择要监控的服务
  selector:
    matchLabels:
      app: seckill-go  # Go服务的标签
```

### 2. RBAC权限配置

在`rbac.yaml`中，我们配置了必要的RBAC权限，确保Prometheus Operator可以访问`couponkill`命名空间中的资源：

- `Role`: 定义了Prometheus Operator需要的权限
- `RoleBinding`: 将Role绑定到Prometheus Operator的ServiceAccount

### 3. 部署步骤

1. 首先确保Prometheus Operator已经在`monitoring`命名空间中部署
2. 在`couponkill`命名空间中部署应用服务
3. 应用RBAC配置：
   ```bash
   kubectl apply -f rbac.yaml
   ```
4. 在`monitoring`命名空间中创建ServiceMonitor资源：
   ```bash
   kubectl apply -f monitoring-resources.yaml
   ```

### 4. 验证配置

部署完成后，可以通过以下方式验证配置：

1. 检查ServiceMonitor是否创建成功：
   ```bash
   kubectl get servicemonitors -n monitoring
   ```

2. 检查Prometheus是否能够发现目标：
   ```bash
   kubectl port-forward -n monitoring svc/prometheus-operated 9090
   ```
   然后访问 http://localhost:9090/targets 查看目标状态

3. 检查是否能够获取到监控指标：
   访问 http://localhost:9090/graph 并查询相关指标

## 注意事项

1. 确保Prometheus Operator已经正确安装并运行在`monitoring`命名空间中
2. 确保应用服务在`couponkill`命名空间中正确部署，并且具有正确的标签
3. RBAC权限配置必须在应用服务所在的命名空间中创建
4. ServiceMonitor资源必须在Prometheus Operator所在的命名空间中创建
5. 确保网络策略允许Prometheus从一个命名空间访问另一个命名空间中的服务