# CouponKill 云原生秒杀系统

## 项目简介

CouponKill 是一个基于云原生技术栈构建的高并发秒杀系统，专为秋招展示而设计。该系统具备高可用性、可扩展性和弹性伸缩能力，能够应对秒杀场景下的流量峰值。

## 技术栈

开发语言: Java、Go
容器编排: Kubernetes
服务网格: Istio
CI/CD: Jenkins
服务注册与发现: Nacos
配置中心: Nacos
分布式事务: Seata
流量控制: Sentinel
消息中间件: RocketMQ
缓存: Redis (阿里云 Redis 服务)
数据库: MySQL (阿里云 RDS 服务)
自动扩缩容: KEDA
监控: 自定义 Operator

##架构图
![架构图.png](docs/%E6%9E%B6%E6%9E%84%E5%9B%BE.png)

##核心服务
couponkill-coupon-service: 优惠券管理服务
couponkill-order-service: 订单服务
couponkill-user-service: 用户服务
couponkill-go-service: 基于 Go 的秒杀核心处理服务
couponkill-gateway: 网关服务
couponkill-operator: 自定义 Kubernetes Operator，用于管理整个秒杀系统的部署和自动扩缩容
## 快速开始

### 环境要求

Kubernetes 1.24+（推荐使用kubekey安装）

Helm 3.0+

Jenkins 2.3+

Docker 2.0.10+

### 部署步骤

1.准备阿里云资源

配置阿里云 RDS MySQL 实例

配置阿里云 Redis 实例

2.部署中间件

#### 部署Nacos

```bash
helm repo add nacos https://nacos-group.github.io/nacos-k8s/
helm install nacos nacos/nacos -n middleware --create-namespace
```


#### 部署Sentinel

```bash
helm repo add sentinel https://sentinelguard.io/helm-charts/
helm install sentinel sentinel/sentinel -n middleware
```

#### 部署RocketMQ

```bash
helm repo add rocketmq https://apache.github.io/rocketmq-externals/helm-charts/
helm install rocketmq rocketmq/rocketmq -n middleware
```

### 部署应用

bash

#### 使用Helm部署应用

```bash
helm install couponkill ./charts/couponkill -n couponkill --create-namespace
```

#### 部署Operator

```bash
kubectl apply -f config/crd/bases/ops.couponkill.io_seckills.yaml
kubectl apply -f config/rbac/
kubectl apply -f config/manager/

```
###或者使用 Makefile:

```bash
cd couponkill-operator
make deploy
```
开发指南
构建 Operator
生成代码和清单:
```bash
   make generate
   make manifests
```
运行 Operator:
```bash
   make docker-build IMG=your-registry/couponkill-operator:tag
   
```
推送镜像
```bash
   make docker-push IMG=your-registry/couponkill-operator:tag
   
```
运行测试
```bash
make test
```
本地运行
```bash
make run
```
监控和运维
Operator 提供了以下监控和运维功能：
1.自动扩缩容: 根据 CPU 使用率或 Kafka 消费者延迟自动调整服务副本数
2.健康检查: 定期检查服务的健康状态
3.状态报告: 提供每个服务的详细状态信息
4.事件记录: 记录重要操作和状态变更事件
故障排除
常见问题
1.Operator 无法启动
检查 RBAC 权限配置
确认 CRD 已正确安装
查看 Operator 日志: kubectl logs -n couponkill-operator-system deployment/couponkill-operator-controller-manager
2.服务无法部署
检查 Seckill 资源配置是否正确
确认镜像名称和标签正确
查看相关 Deployment 和 Service 的状态
3.自动扩缩容不工作
确认 HPA 或 KEDA 配置正确
检查 Metrics Server 是否正常运行
验证资源请求和限制是否已设置
