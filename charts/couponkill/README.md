# CouponKill Helm Chart

CouponKill 是一个基于云原生技术栈构建的高并发秒杀系统，专为秋招展示而设计。该系统具备高可用性、可扩展性和弹性伸缩能力，能够应对秒杀场景下的流量峰值。

## 功能特性

### 微服务架构
基于 Spring Cloud Alibaba 和 Go 语言构建的微服务架构，包含以下核心服务：
- 用户服务 (couponkill-user-service)
- 优惠券服务 (couponkill-coupon-service)
- 订单服务 (couponkill-order-service)
- Go 秒杀服务 (seckill-go-service)
- 网关服务 (couponkill-gateway)
- 自定义 Operator (couponkill-operator)

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

### 动态配置和集群支持
支持中间件的动态扩展和集群模式，包括：
- MySQL 集群模式
- MySQL 主从复制模式
- Redis 集群模式
- Redis 哨兵模式
- Kafka 集群模式
- RocketMQ 集群模式
- Nacos 集群模式

### Nacos配置管理
自动初始化Nacos配置，包括：
- 公共配置(common.yaml)
- 用户服务分库分表配置(user-service-sharding.yaml)
- 订单服务分库分表配置(order-service-sharding.yaml)
- 优惠券服务分库分表配置(coupon-service-sharding.yaml)
- 网关路由配置(gateway-routes.yaml)
- Go服务配置(go-service-dev.yaml)

### ShardingSphere 动态规则
支持 ShardingSphere 分库分表规则的动态更新，服务可以实时感知配置变化。

### Nacos外部访问
Nacos支持外部访问，可通过LoadBalancer服务从集群外部访问Nacos控制台，便于配置管理。

### 零停机动态配置更新
支持在不中断服务的情况下动态更新中间件配置：
- 中间件集群节点动态增减
- 分库分表规则动态调整
- 服务配置参数实时生效
- 配置监听器自动同步配置变更

## 系统架构图

![架构图.png](../../docs/%E6%9E%B6%E6%9E%84%E5%9B%BE.png)

## 部署架构

CouponKill Helm Chart 采用模块化设计，可以根据需要启用或禁用特定组件：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            CouponKill Chart                               │
├─────────────────────────────────────────────────────────────────────────────┤
│  核心服务模块                                                               │
│  ├── 网关服务 (couponkill-gateway)                                          │
│  ├── 用户服务 (couponkill-user-service)                                     │
│  ├── 优惠券服务 (couponkill-coupon-service)                                 │
│  ├── 订单服务 (couponkill-order-service)                                    │
│  ├── Go秒杀服务 (seckill-go-service)                                        │
│  └── Operator (couponkill-operator)                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│  依赖服务模块 (可选)                                                        │
│  ├── Nacos (服务注册与配置中心)                                             │
│  ├── RocketMQ (消息队列)                                                    │
│  ├── MySQL (关系型数据库)                                                   │
│  ├── Redis (缓存)                                                           │
│  └── Sentinel (流量控制)                                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  扩展功能模块                                                               │
│  ├── Istio (服务网格)                                                       │
│  ├── KEDA (自动扩缩容)                                                      │
│  ├── Monitoring (监控)                                                      │
│  └── Examples (示例资源)                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 部署

### 前提条件

- Kubernetes 1.27.0+
- Helm 3.0+
- istio 1.18.0
- keda 2.17.0

### 快速开始

```bash
# 添加 Helm 仓库
helm repo add couponkill https://github.com/couponkill/couponkill-cloud-native

# 安装 Chart
helm install couponkill couponkill/couponkill
```

### 基础部署

```bash
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

## 动态配置和集群支持

### 中间件配置模板

系统为常用的中间件提供了配置模板，用户只需填写节点信息和密码即可快速切换到集群模式。配置模板位于Nacos的`middleware-cluster-config.yaml`中。

#### MySQL配置模板

```yaml
middleware:
  mysql:
    # 集群模式配置 (适用于MySQL Group Replication或类似集群方案)
    cluster:
      enabled: false
      # 节点列表格式: host:port
      # 示例: 
      # nodes:
      #   - "mysql-node1:3306"
      #   - "mysql-node2:3306"
      #   - "mysql-node3:3306"
      nodes: []
    
    # 主从复制模式配置
    replication:
      enabled: false
      # 主库配置
      master:
        host: ""
        port: 3306
      # 从库列表
      # slaves:
      #   - host: "mysql-slave1"
      #     port: 3306
      #   - host: "mysql-slave2"
      #     port: 3306
      slaves: []
```

#### Redis配置模板

```yaml
middleware:
  redis:
    # 集群模式配置 (适用于Redis Cluster)
    cluster:
      enabled: false
      # 节点列表格式: host:port
      # 示例:
      # nodes:
      #   - "redis-node1:6379"
      #   - "redis-node2:6379"
      #   - "redis-node3:6379"
      #   - "redis-node4:6379"
      #   - "redis-node5:6379"
      #   - "redis-node6:6379"
      nodes: []
    
    # 哨兵模式配置 (适用于Redis Sentinel)
    sentinel:
      enabled: false
      # 主节点名称
      masterName: "mymaster"
      # 哨兵节点列表格式: host:port
      # 示例:
      # nodes:
      #   - "sentinel1:26379"
      #   - "sentinel2:26379"
      #   - "sentinel3:26379"
      nodes: []
      # 哨兵密码
      password: ""
```

#### RocketMQ配置模板

```yaml
middleware:
  rocketmq:
    cluster:
      enabled: false
      # NameServer节点列表格式: host:port
      # 示例:
      # nameServer:
      #   - "rocketmq-namesrv1:9876"
      #   - "rocketmq-namesrv2:9876"
      nameServer: []
```

#### Kafka配置模板

```yaml
middleware:
  kafka:
    cluster:
      enabled: false
      # Broker节点列表格式: host:port
      # 示例:
      # brokers:
      #   - "kafka-broker1:9092"
      #   - "kafka-broker2:9092"
      #   - "kafka-broker3:9092"
      brokers: []
```

### 中间件集群模式

CouponKill Helm Chart 支持多种中间件的集群模式，可以通过修改 values.yaml 文件中的配置来启用：

#### Nacos 集群模式

```yaml
nacos:
  cluster:
    enabled: true
    replicas: 3
```

#### RocketMQ 集群模式

```yaml
rocketmq:
  nameServer:
    cluster:
      enabled: true
      replicas: 3
  broker:
    cluster:
      enabled: true
      replicas: 2
      config:
        brokerRole: ASYNC_MASTER
        flushDiskType: ASYNC_FLUSH
```

#### 数据库集群模式

```yaml
db:
  cluster:
    enabled: true
    nodes:
      - "mysql-0.mysql-headless:3306"
      - "mysql-1.mysql-headless:3306"
      - "mysql-2.mysql-headless:3306"
```

#### 数据库主从复制模式

```yaml
db:
  replication:
    enabled: true
    master:
      host: "mysql-master"
      port: 3306
    slaves:
      - host: "mysql-slave-0"
        port: 3306
      - host: "mysql-slave-1"
        port: 3306
```

#### Redis 集群模式

```yaml
redis:
  cluster:
    enabled: true
    nodes:
      - "redis-0.redis-headless:6379"
      - "redis-1.redis-headless:6379"
      - "redis-2.redis-headless:6379"
```

#### Redis 哨兵模式

```yaml
redis:
  sentinel:
    enabled: true
    masterName: "mymaster"
    nodes:
      - "redis-sentinel-0.redis-headless:26379"
      - "redis-sentinel-1.redis-headless:26379"
      - "redis-sentinel-2.redis-headless:26379"
```

### 动态配置更新

所有服务都支持通过 Nacos 进行动态配置更新。当在 Nacos 中修改配置后，服务会自动感知并应用新的配置，无需重启服务。

#### 分库分表规则动态更新

ShardingSphere 的分库分表规则可以通过 Nacos 进行动态更新。更新规则后，服务会自动重新加载规则并应用到新的数据访问中。

#### 零停机配置变更

系统支持以下零停机配置变更：

1. **中间件节点动态增减**：
   - 在 Nacos 配置中添加或删除中间件节点
   - 服务自动感知节点变化并调整连接策略

2. **分库分表规则调整**：
   - 修改分库分表规则配置
   - 服务实时应用新规则，无需重启

3. **服务配置参数更新**：
   - 调整线程池大小、连接池配置等参数
   - 配置实时生效

4. **配置监听器**：
   - 系统内置配置监听器，定期检查并同步配置变更
   - 可通过 `nacos.configWatcher.schedule` 配置检查频率

### Nacos 集群模式和动态配置

Nacos 本身支持集群模式部署，通过 ConfigMap 管理集群配置，可以动态添加或删除节点。当启用 Nacos 集群模式时，系统会自动创建 Headless Service 和 StatefulSet 来管理 Nacos 集群节点。

#### 启用 Nacos 集群模式

在 values.yaml 中设置：

```yaml
nacos:
  cluster:
    enabled: true
    replicas: 3
```

#### 动态更新 Nacos 集群配置

1. 修改 values.yaml 中的 Nacos 集群配置
2. 执行 `helm upgrade` 命令更新部署
3. Nacos 集群会自动更新配置并重新加载

#### 中间件集群配置管理

通过 Nacos 管理中间件集群配置，可以在运行时动态启用或禁用集群模式：

1. 登录 Nacos 控制台
2. 找到 `middleware-cluster-config.yaml` 配置
3. 修改配置内容，例如启用 Redis 集群模式：

```yaml
middleware:
  redis:
    cluster:
      enabled: true
      nodes:
        - "redis-0.redis-headless:6379"
        - "redis-1.redis-headless:6379"
        - "redis-2.redis-headless:6379"
```

4. 保存配置后，相关服务会自动感知并应用新的集群配置

### 网关路由配置管理

系统支持通过 Nacos 动态管理网关路由配置：

1. 登录 Nacos 控制台
2. 找到 `gateway-routes.yaml` 配置
3. 修改路由规则
4. 保存配置后，网关会自动感知并应用新的路由规则

### Go服务配置管理

Go服务支持通过 Nacos 进行动态配置管理：

1. 登录 Nacos 控制台
2. 找到 `go-service-dev.yaml` 配置
3. 修改服务配置（如端口、Redis配置等）
4. 保存配置后，Go服务会自动感知并应用新的配置

### 自动扩缩容

通过 KEDA 实现基于消息队列的自动扩缩容：

```yaml
keda:
  enabled: true
  kafka:
    bootstrapServers: "broker:9092"
    go:
      enabled: true
      deploymentName: "seckill-go-svc"
      consumerGroup: "seckill-go-group"
      topic: "seckill_order_create"
      minReplicaCount: 1
      maxReplicaCount: 5
      lagThreshold: "5"
```

## 零停机集群扩展

系统支持在不中断服务的情况下进行集群扩展，包括中间件集群扩展和Nacos自身集群化：

### 中间件集群扩展

1. 在Nacos控制台中修改`middleware-cluster-config.yaml`配置
2. 添加新的节点信息并启用集群模式
3. 服务会自动感知新的集群配置并建立连接

例如，扩展Redis集群：
```yaml
middleware:
  redis:
    cluster:
      enabled: true
      nodes:
        - "redis-0.redis-headless:6379"
        - "redis-1.redis-headless:6379"
        - "redis-2.redis-headless:6379"  # 新增节点
```

### Nacos自身集群化

1. 初始部署时Nacos以单机模式运行
2. 当需要扩展为集群时，修改values.yaml：
   ```yaml
   nacos:
     cluster:
       enabled: true
       replicas: 3
   ```
3. 执行`helm upgrade`更新部署
4. Nacos会自动切换到集群模式，服务会自动连接到新的集群

## Istio服务网格

CouponKill Helm Chart 支持 Istio 服务网格功能，提供流量管理、安全性和可观察性。

### 基础Istio功能

通过设置 `istio.enabled=true` 启用基础 Istio 功能：
- 自动注入 Envoy sidecar 代理
- 基本的流量路由和负载均衡
- 服务间 mTLS 加密通信
- 基础的监控指标收集

### 高级Istio功能

项目还提供了位于 [k8s-istio](../../k8s-istio) 目录下的完整 Istio 配置，包含以下高级功能：

1. **细粒度流量管理**
   - VirtualService：精确控制服务路由规则
   - DestinationRule：配置流量策略和故障处理
   - Gateway：管理入口流量

2. **安全控制**
   - AuthorizationPolicy：服务访问授权控制
   - PeerAuthentication：服务间认证策略
   - ServiceEntry：管理对外部服务的访问

3. **可观察性**
   - Telemetry：自定义遥测配置
   - Sidecar：优化服务间通信

4. **高级网络功能**
   - EnvoyFilter：自定义 Envoy 代理配置
   - 故障注入和延迟注入测试

### 部署高级Istio功能

要部署完整的 Istio 功能，请执行以下步骤：

```bash
# 部署基础Istio组件（如果尚未部署）
# 参考Helm Charts中的istio.enabled=true配置

# 部署高级Istio配置
kubectl apply -f ../../k8s-istio/
```

这将应用所有高级 Istio 配置，包括细粒度的流量管理、安全策略和可观察性配置。

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