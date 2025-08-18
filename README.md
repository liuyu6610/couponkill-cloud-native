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
## 核心服务
couponkill-coupon-service: 优惠券管理服务

couponkill-order-service: 订单服务

couponkill-user-service: 用户服务

couponkill-go-service: 基于 Go 的秒杀服务协同java的秒杀服务

couponkill-gateway: 网关服务

## 快速开始

### 环境要求

Kubernetes 1.24+（推荐使用kubekey安装）

Helm 3.0+

Jenkins 2.300+

Docker 2.0.10+

### 部署步骤

准备阿里云资源

配置阿里云 RDS MySQL 实例

配置阿里云 Redis 实例

部署步骤

准备阿里云资源

配置阿里云 RDS MySQL 实例

配置阿里云 Redis 实例

部署中间件

#### 部署Nacos

```bash
helm repo add nacos https://nacos-group.github.io/nacos-k8s/
helm install nacos nacos/nacos -n middleware --create-namespace
```

#### 部署Seata

```bash
helm repo add seata https://seata.io/seata-helm-charts/
helm install seata seata/seata -n middleware
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
kubectl apply -f k8s/operator-deployment.yaml
```

