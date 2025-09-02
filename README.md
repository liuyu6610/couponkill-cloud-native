# CouponKill 云原生秒杀系统

## 项目简介

CouponKill 是一个基于云原生技术栈构建的高并发秒杀系统，专为秋招展示而设计。该系统具备高可用性、可扩展性和弹性伸缩能力，能够应对秒杀场景下的流量峰值。

## 补充说明

开发的过程中有随便试试的基础版本，但是由于时间原因，没有继续开发，所以现在的版本是一个基础版本。对于前端在k8s-istio设置了完整的规则包含core,helm一
键部署没有使用自定义的istio,如果想要使用自定义的istio,只需要改动一下即可，二者是兼容的，本人已经测试过，helm新增了serviceAccount，
并绑定了istio的角色，这样就可以使用自定义的istio了。一键部署不需要那么复杂的istio就简化了.

监控功能和keda建议不要全部署在couponkill空间，并且都需要提前部署好，否则会导致部署失败。
非必须的功能有很多，比如istio,监控功能，operator,sentinel-dashboard,prometheus,kafka

还有简单般的k8s-nothing的版本，只有最最基本的版本，就是项目需要的各种配置没有任何其他额外功能。

## 技术栈

开发语言: Java、Go

容器编排: Kubernetes

服务网格: Istio

CI/CD: Jenkins和云效

服务注册与发现: Nacos

配置中心: Nacos

分布式事务: Seata（最开始使用了之后引入分库分表去除了，shardingSphere自带的有事务模式）

流量控制: Sentinel

消息中间件: RocketMQ

缓存: Redis (阿里云 Redis 服务)

数据库: MySQL (阿里云 RDS 服务)

自动扩缩容: KEDA

监控: 自定义 Operator

## 架构图

![架构图.png](docs/%E6%9E%B6%E6%9E%84%E5%9B%BE.png)

## 核心服务

- **couponkill-coupon-service**: 优惠券管理服务
- **couponkill-order-service**: 订单服务
- **couponkill-user-service**: 用户服务
- **couponkill-go-service**: 基于 Go 的秒杀核心处理服务
- **couponkill-gateway**: 网关服务
- **couponkill-operator**: 自定义 Kubernetes Operator，用于管理整个秒杀系统的部署和自动扩缩容

## 高并发优化策略

### Redis缓存优化
- 使用Redis作为主要缓存层，存储热点数据如用户信息、优惠券详情等
- 采用连接池管理Redis连接，避免频繁创建和销毁连接的开销
- 实现缓存预热机制，在系统启动时加载热点数据到缓存中
- 使用Redis分布式锁保证库存扣减的原子性操作
- 通过Lua脚本实现原子性的库存扣减和订单创建操作

### 分库分表策略
- 用户表按用户ID进行分库分表，分散存储压力
- 订单表按用户ID进行分库分表，提高查询效率
- 优惠券表按分片索引进行分库分表，支持海量数据存储
- 通过ShardingSphere实现分库分表的路由和管理
- 实现全局唯一ID生成器，保证分库分表环境下的ID唯一性

### RocketMQ消息队列
- 使用RocketMQ异步处理订单创建、库存扣减等非核心流程
- 实现消息的顺序消费，保证数据一致性
- 通过消息队列削峰填谷，缓解瞬时高并发压力
- 实现消息补偿机制，确保消息的可靠传递
- 支持消息的批量发送和消费，提高处理效率

### 线程池优化
- 配置合理的线程池参数，包括核心线程数、最大线程数、队列容量等
- 针对不同类型的任务配置独立的线程池，避免任务相互影响
- 实现线程池的动态调参，根据系统负载自动调整线程池参数
- 使用CompletableFuture并行处理多个互不依赖的任务，提高处理效率
- 实现异步处理机制，将非核心业务逻辑异步化，减少主线程阻塞

### Gateway网关优化
- 使用Spring Cloud Gateway作为统一入口，实现请求路由、负载均衡等功能
- 集成Sentinel实现网关层的流量控制和熔断降级
- 实现JWT认证鉴权，保证系统安全
- 支持动态路由配置，通过Nacos实现路由规则的动态更新
- 实现请求限流，防止系统被突发流量击垮

### Go与Java协同处理
- Java服务处理复杂业务逻辑，提供稳定可靠的服务
- Go服务处理高并发场景下的和java模块创建订单的全部流程，包括库存扣减、订单创建等，发挥Go语言高并发优势
- 通过Nacos实现服务发现和服务配置的统一管理
- 实现智能负载分配，当Java服务达到处理能力上限时，自动将额外请求路由到Go服务
- 通过RocketMQ实现服务间异步通信，降低服务间耦合度

## 快速开始

### 环境要求

- Kubernetes:1.27+（推荐使用kubekey安装）
- Helm:3.0+
- istio: 1.18.0
- Jenkins:2.3+（可选，更推荐相关产品，如阿里云的云效，现在相关产品很完善并且可以利用可视化界面进行配置优化）
- Docker:20.10+
- kubectl:1.27+
### 部署步骤

#### 一键部署（推荐）

使用 Helm Chart 一键部署整个系统：

```bash
# 普通部署
helm install couponkill ./charts/couponkill --namespace couponkill --create-namespace

# 生产环境部署
helm install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values-prod.yaml

# 金丝雀发布部署
helm install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values.canary-keda.yaml
```

#### 手动部署中间件

##### 部署 Nacos

```bash
helm repo add nacos https://nacos-group.github.io/nacos-k8s/
helm install nacos nacos/nacos -n middleware --create-namespace
```

##### 部署 Sentinel

```bash
helm repo add sentinel https://sentinelguard.io/helm-charts/
helm install sentinel sentinel/sentinel -n middleware
```

##### 部署 RocketMQ

```bash
helm repo add rocketmq https://apache.github.io/rocketmq-externals/helm-charts/
helm install rocketmq rocketmq/rocketmq -n middleware
```

### 部署应用

#### 使用 Helm 部署应用

```bash
# 稳定版本部署
helm install couponkill ./charts/couponkill -n couponkill --create-namespace

# 金丝雀版本部署
helm install couponkill-canary ./charts/couponkill -n couponkill --create-namespace -f ./charts/couponkill/values.canary-keda.yaml
```

#### 部署 Operator

```bash
# 安装 CRD
kubectl apply -f couponkill-operator/config/crd/bases/

# 安装 RBAC 权限
kubectl apply -f couponkill-operator/config/rbac/

# 部署 Operator
kubectl apply -f couponkill-operator/config/manager/
```

或者使用 Makefile:

```bash
cd couponkill-operator
make deploy
```

## 开发指南

### 构建 Operator

生成代码和清单:
```bash
make generate
make manifests
```

运行 Operator:
```bash
make run
```

### 构建 Docker 镜像

```bash
# 构建所有镜像
make build-all-images

# 构建并推送稳定版镜像到阿里云仓库
make build-and-push-all

# 构建并推送金丝雀版本镜像到阿里云仓库
make build-and-push-all-canary

# 或者单独构建每个服务的镜像
docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
docker build -t order:latest -f couponkill-order-service/Dockerfile .
docker build -t user:latest -f couponkill-user-service/Dockerfile .
docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
docker build -t operator:latest -f couponkill-operator/Dockerfile .
```

### 运行测试

```bash
make test
```

## 构建和部署脚本说明

CouponKill 项目提供了多种方式来构建和部署系统，包括 Makefile 和 PowerShell 脚本。

### Makefile 脚本（Linux/macOS）

在类 Unix 系统上，可以使用 Makefile 来执行各种构建和部署任务：

```bash
# 显示所有可用命令
make help

# 构建所有服务的 Docker 镜像
make build-all-images

# 部署整个系统
make deploy-chart

# 生产环境部署
make deploy-chart-prod

# 金丝雀发布部署
make deploy-chart-canary

# 构建并推送所有镜像到注册表
make build-and-push-all

# 构建金丝雀版本镜像
make build-all-images-canary

# 推送金丝雀版本镜像
make push-all-images-canary

# 构建并推送所有金丝雀镜像
make build-and-push-all-canary

# 拉取依赖镜像
make pull-dependency-images

# 构建并推送所有镜像（包括依赖）
make build-and-push-all-complete
```

### PowerShell 脚本（Windows）

在 Windows 系统上，可以使用 PowerShell 脚本来执行构建和部署任务：

```powershell
# 显示帮助信息
powershell -File build.ps1 help

# 构建所有服务的 Docker 镜像
powershell -File build.ps1 build-all-images

# 部署整个系统
powershell -File build.ps1 deploy-chart

# 生产环境部署
powershell -File build.ps1 deploy-chart-prod

# 金丝雀发布部署
powershell -File build.ps1 deploy-chart-canary

# 构建并推送所有镜像到注册表
powershell -File build.ps1 build-and-push-all

# 构建金丝雀版本镜像
powershell -File build.ps1 build-all-images-canary

# 推送金丝雀版本镜像
powershell -File build.ps1 push-all-images-canary

# 构建并推送所有金丝雀镜像
powershell -File build.ps1 build-and-push-all-canary

# 拉取依赖镜像
powershell -File build.ps1 pull-dependency-images

# 构建并推送所有镜像（包括依赖）
powershell -File build.ps1 build-and-push-all-complete
```

或者在 PowerShell 中直接运行：

```powershell
# 导航到项目根目录
cd D:\couponkill\couponkill-cloud-native

# 直接运行脚本
.\build.ps1 build-all-images
```

### 脚本功能对照表

| 功能 | Makefile 命令 | PowerShell 命令 | 说明 |
|------|---------------|-----------------|------|
| 构建所有镜像 | `make build-all-images` | `build.ps1 build-all-images` | 构建所有服务的 Docker 镜像 |
| 部署系统 | `make deploy-chart` | `build.ps1 deploy-chart` | 使用 Helm Chart 部署整个系统 |
| 生产部署 | `make deploy-chart-prod` | `build.ps1 deploy-chart-prod` | 生产环境部署 |
| 金丝雀部署 | `make deploy-chart-canary` | `build.ps1 deploy-chart-canary` | 金丝雀发布部署 |
| 构建并推送镜像 | `make build-and-push-all` | `build.ps1 build-and-push-all` | 构建并推送稳定版镜像 |
| 构建金丝雀镜像 | `make build-all-images-canary` | `build.ps1 build-all-images-canary` | 构建金丝雀版本镜像 |
| 推送金丝雀镜像 | `make push-all-images-canary` | `build.ps1 push-all-images-canary` | 推送金丝雀版本镜像 |
| 构建并推送金丝雀镜像 | `make build-and-push-all-canary` | `build.ps1 build-and-push-all-canary` | 构建并推送金丝雀版本镜像 |
| 拉取依赖镜像 | `make pull-dependency-images` | `build.ps1 pull-dependency-images` | 拉取依赖服务镜像 |
| 完整构建推送 | `make build-and-push-all-complete` | `build.ps1 build-and-push-all-complete` | 构建并推送所有镜像 |

## 监控和运维

Operator 提供了以下监控和运维功能：

1. 自动扩缩容: 根据 CPU 使用率或 Kafka 消费者延迟自动调整服务副本数
2. 健康检查: 定期检查服务的健康状态
3. 状态报告: 提供每个服务的详细状态信息
4. 事件记录: 记录重要操作和状态变更事件

## 金丝雀发布

CouponKill 系统支持金丝雀发布，可以通过 Helm Chart 部署稳定版本和金丝雀版本，实现渐进式交付。

### 金丝雀发布配置

金丝雀发布配置在 `charts/couponkill/values.canary-keda.yaml` 文件中定义：

- 使用独立的阿里云镜像仓库 `crpi-n5rumpjwbqinoz4c-vpc.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/canary-keda-dev` 存储金丝雀版本镜像
- 为各个服务配置流量权重，控制稳定版本和金丝雀版本之间的流量分配
- 支持基于 KEDA 的自动扩缩容

### 部署金丝雀版本

```bash
# 部署金丝雀版本
helm install couponkill-canary ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values.canary-keda.yaml

# 更新金丝雀版本配置
helm upgrade couponkill-canary ./charts/couponkill --namespace couponkill -f ./charts/couponkill/values.canary-keda.yaml

# 动态调整流量权重（通过 Nacos 或其他配置中心）
# 可以在 Nacos 中修改服务权重配置，实现动态流量切换
```

## 故障排除

### 常见问题

1. Operator 无法启动
   - 检查 RBAC 权限配置
   - 确认 CRD 已正确安装
   - 查看 Operator 日志: `kubectl logs -n couponkill deployment/couponkill-operator`

2. 服务无法部署
   - 检查 Seckill 资源配置是否正确
   - 确认镜像名称和标签正确
   - 查看相关 Deployment 和 Service 的状态

3. 自动扩缩容不工作
   - 确认 HPA 或 KEDA 配置正确
   - 检查 Metrics Server 是否正常运行
   - 验证资源请求和限制是否已设置

4. 金丝雀发布问题
   - 检查镜像仓库地址和标签是否正确
   - 确认流量权重配置是否合理
   - 查看 Istio 配置是否正确应用

### 访问服务

部署完成后，可以通过以下方式访问服务：

- 网关服务: `http://<ingress-ip>`
- 用户服务: `http://<ingress-ip>/api/v1/user`
- 订单服务: `http://<ingress-ip>/api/v1/order`
- 优惠券服务: `http://<ingress-ip>/api/v1/coupon`
- Go秒杀服务: `http://<ingress-ip>/seckill`
- Sentinel Dashboard: `http://<ingress-ip>:8080`
- Nacos Dashboard: `http://<ingress-ip>:8848`

## CI/CD 流程

项目使用 云效的devops 进行持续集成和持续部署。并且利用可视化改进jenkins的配置， 定义了完整的构建、测试、打包和部署流程。

主要步骤包括：
1. 并行构建所有服务
2. 构建并推送 Docker 镜像到私有仓库（包括稳定版本和金丝雀版本）
3. 使用 Helm 部署到 Kubernetes 集群
4. 部署金丝雀版本以支持渐进式交付

通过金丝雀发布功能，可以实现更安全、更可控的服务更新，降低新版本上线带来的风险。