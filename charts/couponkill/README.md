# CouponKill Helm Chart

CouponKill秒杀系统Helm Chart，支持一键部署完整系统。

## 简介

CouponKill是一个基于云原生微服务架构的秒杀系统，包含以下组件：
- 网关服务 (Gateway Service)
- 用户服务 (User Service)
- 优惠券服务 (Coupon Service)
- 订单服务 (Order Service)
- Go秒杀服务 (Go Seckill Service)
- Operator控制器 (CouponKill Operator)
- RocketMQ消息队列
- Nacos服务发现与配置中心
- Sentinel流量防护组件

## 版本信息

- Chart版本: 0.9.0
- 应用版本: 1.7.0

## 先决条件

- Kubernetes 1.16+
- Helm 3.0+
- Istio 1.10+ (可选)
- KEDA 2.0+ (可选)
- Prometheus & Grafana (可选)

## 安装Chart

### 一键部署完整系统

```bash
# 添加repo (如果从远程repo安装)
helm repo add couponkill https://couponkill.github.io/couponkill-cloud-native

# 本地安装
helm install couponkill ./couponkill
```

### 生产环境部署

```bash
helm install couponkill ./couponkill -f ./couponkill/values-prod.yaml
```

### 自定义部署

```bash
helm install couponkill ./couponkill \
  --set image.tag=v1.0.0 \
  --set services.go.replicas=3 \
  --set services.coupon.replicas=3
```

## 配置参数

以下表格列出了Chart支持的配置参数：

| 参数 | 描述 | 默认值 |
|------|------|--------|
| `namespace` | 部署命名空间 | `couponkill` |
| `crd.install` | 是否安装CRD | `true` |
| `examples.seckill` | 是否部署示例资源 | `false` |
| `dependencies.enabled` | 是否部署依赖服务 | `true` |
| `image.registry` | 镜像仓库地址 | `crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker` |
| `image.tag` | 镜像标签 | `latest` |
| `services.gateway.replicas` | 网关服务副本数 | `1` |
| `services.go.replicas` | Go服务副本数 | `2` |
| `services.coupon.replicas` | 优惠券服务副本数 | `2` |
| `services.order.replicas` | 订单服务副本数 | `2` |
| `services.user.replicas` | 用户服务副本数 | `1` |
| `services.operator.replicas` | Operator副本数 | `1` |
| `rocketmq.enabled` | 是否启用RocketMQ | `true` |
| `nacos.enabled` | 是否启用Nacos | `true` |
| `nacos.service.external.enabled` | 是否启用Nacos外部访问 | `true` |
| `sentinel.enabled` | 是否启用Sentinel | `true` |

## 性能优化配置

### JVM优化参数

所有Java服务（Gateway、Coupon、Order、User）都配置了JVM优化参数，包括：
- G1垃圾回收器优化
- 对象指针压缩
- 字符串连接优化
- 大页内存使用

### 线程池优化

各服务配置了专门的线程池参数以适应不同的负载需求：
- 网关服务：最大线程数500，用于处理高并发请求
- 订单服务：最大线程数400，处理复杂的订单逻辑
- 优惠券服务：最大线程数300，处理优惠券相关操作
- 用户服务：最大线程数200，处理用户相关操作

### Go运行时优化

Go服务（Go Seckill和Operator）配置了运行时优化参数：
- GOGC=20：降低垃圾回收阈值，更频繁地进行垃圾回收以减少内存占用
- GOMAXPROCS=4：限制CPU使用核心数，避免过度占用系统资源

## 升级Chart

```bash
helm upgrade couponkill ./couponkill
```

## 卸载Chart

```bash
helm uninstall couponkill
```

## 功能特性

### 服务网格支持 (Istio)
Chart支持Istio服务网格，提供流量管理、安全性和可观察性功能。

### 自动扩缩容 (KEDA)
通过KEDA实现基于Kafka消息队列的自动扩缩容功能。

### 监控集成
支持Prometheus和Grafana监控集成。

### 数据库初始化
自动创建数据库表结构。

### 依赖服务管理
支持MySQL、Redis、Kafka、RocketMQ、Nacos和Sentinel等依赖服务的部署和初始化。

### Nacos配置管理
自动初始化Nacos配置，包括：
- 公共配置(common.yaml)
- 用户服务分库分表配置(user-service-sharding.yaml)
- 订单服务分库分表配置(order-service-sharding.yaml)
- 优惠券服务分库分表配置(coupon-service-sharding.yaml)

### Nacos外部访问
Nacos支持外部访问，可通过LoadBalancer服务从集群外部访问Nacos控制台，便于配置管理。

## 访问服务

部署完成后，可以通过以下方式访问服务：

- 网关服务: `http://<ingress-ip>`
- 用户服务: `http://<ingress-ip>/api/v1/user`
- 订单服务: `http://<ingress-ip>/api/v1/order`
- 优惠券服务: `http://<ingress-ip>/api/v1/coupon`
- Go秒杀服务: `http://<ingress-ip>/seckill`
- Sentinel Dashboard: `http://<ingress-ip>:8080`
- Nacos Dashboard: `http://<external-ip>:8848` (外部访问)

## 故障排除

### 查看服务状态

```bash
kubectl get pods -n couponkill
kubectl get services -n couponkill
```

### 查看日志

```bash
kubectl logs -n couponkill <pod-name>
```

### 查看事件

```bash
kubectl get events -n couponkill
```