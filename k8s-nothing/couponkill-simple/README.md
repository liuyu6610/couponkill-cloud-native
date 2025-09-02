# CouponKill 简易版部署说明

## 概述

CouponKill 简易版是一个简化部署版本，将整个系统分为三个主要部分：

1. **业务服务Pod** - 包含所有业务服务（网关、订单、优惠券、用户、Go服务）
2. **中间件Pod** - 包含所有中间件服务（Nacos、RocketMQ、Sentinel）
3. **存储Pod** - 包含数据库和缓存服务（MySQL、Redis）以及持久化存储

> 注意：Operator组件在简化版中不启用，因此已从部署配置中移除

## 部署架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Ingress Controller                     │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                       Ingress                               │
│  /api/v1/user       →  Gateway (8080)                      │
│  /api/v1/order      →  Gateway (8080)                      │
│  /api/v1/coupon     →  Gateway (8080)                      │
│  /seckill           →  Gateway (8080)                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                    couponkill-service                      │
│  Gateway (8080)  →  各业务服务                              │
│     │                                                       │
│     ├── User Service (8083)                                │
│     ├── Order Service (8082)                               │
│     ├── Coupon Service (8081)                              │
│     └── Go Service (8090)                                  │
└─────────────────────────────────────────────────────────────┘

┌──────────────────┐    ┌──────────────────────┐
│ software-service │    │  storage-service     │
│                  │    │                      │
│ Nacos (8848)     │    │ MySQL (3306)         │
│ RocketMQ (9876)  │    │ Redis (6379)         │
│ Sentinel (8080)  │    │                      │
└──────────────────┘    └──────────────────────┘
```

## 部署步骤

通过以下三个命令即可完成整个系统的部署：

```bash
# 1. 创建命名空间和密钥
kubectl apply -f couponKill.yaml

# 2. 部署存储服务和中间件服务
kubectl apply -f storage-ware.yaml
kubectl apply -f coupon-ware.yaml

# 3. 创建服务和路由
kubectl apply -f coupon-service.yaml
kubectl apply -f coupon-ingress.yaml
kubectl apply -f coupon-config.yaml
```

## 服务访问

部署完成后，可以通过以下方式访问服务：

- **网关服务**: `http://<ingress-ip>/api/v1/`
  - 用户服务: `http://<ingress-ip>/api/v1/user`
  - 订单服务: `http://<ingress-ip>/api/v1/order`
  - 优惠券服务: `http://<ingress-ip>/api/v1/coupon`
  - Go秒杀服务: `http://<ingress-ip>/seckill`

- **直接访问各服务**（用于测试）:
  - 网关: `http://<node-ip>:<node-port>/gateway`
  - 用户服务: `http://<node-ip>:<node-port>/user`
  - 订单服务: `http://<node-ip>:<node-port>/order`
  - 优惠券服务: `http://<node-ip>:<node-port>/coupon`
  - Go服务: `http://<node-ip>:<node-port>/go`

## 配置说明

本部署方案通过Kubernetes Service实现服务发现，各模块只需要知道Nacos的地址即可，不需要复杂的配置：

- **Nacos服务地址**: `software-service:8848`
- **RocketMQ服务地址**: `software-service:9876`
- **MySQL服务地址**: `storage-service:3306`
- **Redis服务地址**: `storage-service:6379`

这些配置已经通过ConfigMap提供给各服务，服务启动时会自动从Nacos获取详细配置。

### 镜像仓库

所有服务使用阿里云ACR私有镜像仓库：
```
crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker
```

### 端口映射

各服务默认端口：
- Gateway: 8080
- Coupon Service: 8081
- Order Service: 8082
- User Service: 8083
- Go Service: 8090
- Nacos: 8848
- RocketMQ Namesrv: 9876
- RocketMQ Broker: 10911
- Sentinel Dashboard: 8080
- MySQL: 3306
- Redis: 6379

## 注意事项

1. 确保Kubernetes集群可以访问阿里云镜像仓库
2. 确保节点有足够资源运行所有服务
3. 首次部署时，中间件和存储服务可能需要一些时间初始化
4. 可以通过`kubectl get pods -n couponkill`查看Pod状态
5. 通过`kubectl logs -n couponkill <pod-name> -c <container-name>`查看容器日志
6. 网关作为统一入口，负责将请求路由到相应的后端服务
7. 服务间通过software-service和storage-service进行通信
8. 所有配置通过ConfigMap提供最基本的服务发现地址，详细配置由Nacos管理