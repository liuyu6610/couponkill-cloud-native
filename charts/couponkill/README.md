# charts/couponkill

CouponKill 的 **唯一** Kubernetes 部署入口：五个 Go 服务（gateway、user、coupon、order、connector）+ 可选的集群内 PostgreSQL 16 / Redis 7 / Kafka（KRaft）。
设计依据见 [`docs/GO-REWRITE-DESIGN.md`](../../docs/GO-REWRITE-DESIGN.md)（§3 配置、§8 Redis 键、§9 主题与消费组、§12 交付）。

与旧（Java）版本的区别：

* 配置**只有环境变量**：非机密进 ConfigMap（`CK_*`），机密进 Secret（`secretKeyRef`）。没有 Nacos / Sentinel / ShardingSphere。
* **没有 db-init Job**：每个服务启动时在 advisory lock 下迁移自己的 schema（`usr` `cpn` `ord` `cnx`）。
* 唯一的 Job 是 Kafka 主题初始化（`seckill_order_create`、`seckill_order_create_dlt`）。
* 镜像命名统一为 `<registry>/couponkill-<svc>:<tag>`，由 `build/Dockerfile` 构建（distroless，非 root，只读根文件系统）。

## 安装

前置：Kubernetes ≥ 1.27、Helm 3、默认 StorageClass（用自带 PostgreSQL/Redis/Kafka 且开启持久化时）。

```bash
# 开发 / 演示：chart 生成 Secret（随机密钥），依赖全部自带
helm upgrade --install couponkill charts/couponkill -n couponkill --create-namespace \
  --set-string image.tag=<标签>

# 本机访问网关
kubectl -n couponkill port-forward svc/couponkill-gateway 8088:80
curl -s http://127.0.0.1:8088/healthz
```

演示数据（用户 `demo/demo1234`、`admin/admin1234`，秒杀券 + 常驻券）：`--set config.seedDemo=true`，**仅限开发**（`config.env=prod` 时渲染失败）。

### 生产

```bash
# 1. 先创建 Secret（chart 不为生产生成密钥；config.env=prod 与 secrets.create=true 同时出现会渲染失败）
kubectl create namespace couponkill
kubectl -n couponkill create secret generic couponkill-app-secrets \
  --from-literal=jwt-secret="$(openssl rand -hex 32)" \
  --from-literal=internal-token="$(openssl rand -hex 24)" \
  --from-literal=gateway-sign-key="$(openssl rand -hex 32)" \
  --from-literal=postgres-password="$(openssl rand -hex 16)" \
  --from-literal=redis-password="$(openssl rand -hex 16)"

# 2. 部署（生产必须指定镜像标签，不接受 latest）
helm upgrade --install couponkill charts/couponkill -n couponkill \
  -f charts/couponkill/values-prod.yaml --set-string image.tag=<构建号>
```

`values-prod.yaml` 假定集群已装 Istio、KEDA、prometheus-operator；缺哪个把对应开关（`istio.enabled`、`keda.enabled`、`monitoring.enabled`）改回 `false`。
使用 Istio 时给命名空间打标签并叠加 [`k8s-istio/`](../../k8s-istio)（授权策略、STRICT mTLS）：

```bash
kubectl label namespace couponkill istio-injection=enabled --overwrite
```

## Secret

| 键（`secrets.keys.*` 可改名） | 注入为 | 谁需要 | 要求 |
|---|---|---|---|
| `jwt-secret` | `CK_JWT_SECRET` | gateway、user | ≥ 32 字节 |
| `internal-token` | `CK_INTERNAL_TOKEN` | user、coupon、order、connector | ≥ 16 字节 |
| `gateway-sign-key` | `CK_GATEWAY_SIGN_KEY` | gateway、user、coupon、order、connector | ≥ 32 字节 |
| `postgres-password` | `CK_PG_PASSWORD`（拼进 `CK_PG_DSN`）；自带 PostgreSQL 的 `POSTGRES_PASSWORD` | user、coupon、order、connector | **URL 安全字符**（`openssl rand -hex`）；含特殊字符时改用 `pg-dsn` |
| `pg-dsn` | `CK_PG_DSN`（整条） | 同上 | 仅 `postgres.dsnFromSecret=true` |
| `redis-password` | `CK_REDIS_PASSWORD`；自带 Redis 的 `requirepass` | 全部服务 | 仅 `redis.auth.enabled=true` |
| `connector-<平台>-…` | 见 `services.connector.secretEnv` | connector | 全部 optional：缺凭证即禁用该平台 |

* 取值**最小权限注入**：gateway 没有数据库口令，coupon/order/connector 没有 JWT 密钥。
* `secrets.create=true`（默认，仅开发）：用 `randAlphaNum` 生成，升级时 `lookup` 保留已有取值（口令变了会与已初始化的 PostgreSQL 数据卷对不上）。
* **轮换**：改 Secret 后 `kubectl -n couponkill rollout restart deploy`（Secret 内容不参与滚动重启的 checksum，ConfigMap 变化才会自动滚动）。轮换 `jwt-secret` 会使所有登录失效；轮换 `gateway-sign-key` 需要同时重启所有服务（网关与下游的密钥必须一致，否则下游拒绝身份头）。

## 每个服务拿到的环境变量

ConfigMap `couponkill-<svc>-config` 给非机密项，容器 `env` 给机密项。✓ = 注入。

| 变量 | gateway | user | coupon | order | connector |
|---|:-:|:-:|:-:|:-:|:-:|
| `CK_ENV` `CK_HTTP_PORT` `CK_LOG_LEVEL` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `CK_REDIS_ADDR` `CK_REDIS_DB` + `CK_REDIS_PASSWORD`(Secret) | ✓ | ✓ | ✓ | ✓ | ✓ |
| `CK_USER_URL` `CK_COUPON_URL` `CK_ORDER_URL` `CK_CONNECTOR_URL` | ✓ |  | ✓ | ✓ | ✓ |
| `CK_PG_DSN`（`CK_PG_PASSWORD`(Secret) 拼出） |  | ✓ | ✓ | ✓ | ✓ |
| `CK_KAFKA_BROKERS` |  |  |  | ✓ |  |
| `CK_JWT_ISSUER` `CK_JWT_AUDIENCE` `CK_JWT_TTL` + `CK_JWT_SECRET`(Secret) | ✓ | ✓ |  |  |  |
| `CK_GATEWAY_SIGN_KEY`(Secret) | ✓ | ✓ | ✓ | ✓ | ✓ |
| `CK_INTERNAL_TOKEN`(Secret) |  | ✓ | ✓ | ✓ | ✓ |
| `CK_SEED_DEMO` |  | ✓ | ✓ |  |  |
| `services.connector.secretEnv`（`CK_CONNECTOR_*`，optional） |  |  |  |  | ✓ |

服务私有的开关写在 `services.<svc>.config`（空字符串 = 不注入，用服务内置默认值），数值请加引号：

| 服务 | 变量 | 含义 |
|---|---|---|
| gateway | `CK_GATEWAY_CORS_ORIGINS` | 允许的前端 Origin，逗号分隔，不能是 `*` |
| gateway | `CK_GATEWAY_TRUSTED_PROXIES` | 信任 `X-Forwarded-For` 的代理 CIDR。放在 Ingress/Istio 之后**必须**填，否则限流把所有请求当成同一个 IP |
| gateway | `CK_GATEWAY_REVOCATION_FAIL_OPEN` | Redis 会话查询失败时放行（默认 true） |
| gateway | `CK_GATEWAY_IP_RPS` `CK_GATEWAY_USER_RPS` `CK_GATEWAY_SECKILL_RPS` | 令牌桶速率 |
| coupon | `CK_COUPON_STOCK_BUCKETS` | 库存分桶数（默认 8），上线后不要调小 |
| order | `CK_ORDER_SECKILL_COOLDOWN` | 秒杀冷却（默认 0s 关闭） |
| order | `CK_ORDER_CONSUME_CONCURRENCY` | 每个副本同时消费的分区数（默认 1；prod 设为 4） |

没有出现在表里的变量，可以用 `services.<svc>.config` 或 `services.<svc>.extraEnv` 追加。
chart 的渲染测试会对照 `internal/platform/config` 校验公共变量是否齐全，并在各服务配置合入后校验这里的变量名在 Go 源码里真实存在。

## 主要 values

完整的带注释默认值见 [`values.yaml`](values.yaml)。

| 区块 | 说明 |
|---|---|
| `image.*` | `registry`、`repositoryPrefix`（`couponkill-`）、`tag`、`pullPolicy`、`pullSecrets`；单服务可用 `services.<svc>.image.{repository,tag}` 覆盖 |
| `config.*` | `env`（dev/prod）、`logLevel`、`seedDemo`、`jwt.{issuer,audience,ttl}` |
| `secrets.*` | `create`、`existingSecret`/`name`、`keys.*`、`values.*`（仅 create=true） |
| `services.<svc>.*` | `replicas`、`resources`、`autoscaling`（HPA）、`config`、`extraEnv`、`nodeSelector`/`tolerations`/`affinity`/`topologySpreadConstraints` |
| `probes.*` `terminationGracePeriodSeconds` | 探针与优雅停机窗口（应用关闭超时默认 25s） |
| `pdb.*` | `maxUnavailable: 1`，单副本也允许节点维护 |
| `postgres.*` `redis.*` `kafka.*` | 自带依赖或外部地址（`enabled=false` + `host` / `brokers`），持久化、资源 |
| `kafka.topics.seckillOrderCreate.partitions` | 默认 16 = order 消费者并行度上限；`topicInit` Job 会把已存在主题的分区数调高到该值 |
| `networkPolicy.*` | default-deny；仅 gateway 接受入口命名空间的流量；外部依赖用 `extraEgress` 放行 |
| `istio.*` `ingress.*` | 二选一的入口，只暴露 gateway |
| `canary.{coupon,order}.*` | 金丝雀 Deployment 与流量权重 |
| `keda.*` | 按 `seckill_order_create` 积压扩缩 order |
| `monitoring.*` | Pod 注解与 ServiceMonitor（`/metrics`） |
| `operator.*` `examples.seckill` | 可选的 Operator / CRD / 示例资源，默认关闭 |

### values 文件

| 文件 | 用途 |
|---|---|
| `values.yaml` | 开发 / 演示默认：自带依赖、chart 生成 Secret、Istio/KEDA/监控全关 |
| `values-prod.yaml` | 生产：`config.env=prod`、预建 Secret、多副本 + HPA、持久卷放大、Istio + KEDA + 监控开启 |
| `values.canary-keda.yaml` | 金丝雀 + KEDA + Istio，可独立渲染，也可叠加在 prod 之后 |

渲染期校验（失败即报错，不会部署出不安全或自相矛盾的配置）：`config.env=prod` 时拒绝 `latest` 标签、拒绝 `seedDemo`、拒绝 `secrets.create=true`；
`istio.enabled` 与 `ingress.enabled` 互斥；KEDA `maxReplicaCount` 不得超过主题分区数；金丝雀权重必须在 0–100；外部依赖必须给地址。

## 探针、资源与安全上下文

* 探针：`/healthz`（存活，不查依赖）、`/readyz`（就绪，查本服务依赖）、`startupProbe` 给启动期迁移留时间（默认 2s × 30）。
* Pod：`runAsNonRoot`、uid/gid 65532、`seccompProfile: RuntimeDefault`；容器：`readOnlyRootFilesystem: true`（挂一个 `/tmp` emptyDir）、`allowPrivilegeEscalation: false`、`drop: [ALL]`；不挂载 ServiceAccount Token。
* 滚动更新 `maxUnavailable: 0`；同一服务的副本软反亲和到不同节点。

## 金丝雀

`canary.coupon.enabled` / `canary.order.enabled` 会为该服务额外起一个 `version=canary` 的 Deployment，与稳定版挂在同一个 Service 下
（`<name>-canary`，镜像标签 `canary.<svc>.tag`）。没有 Istio 时按副本比例分流；`istio.enabled=true` 时 `<name>-canary-vs` 按 `weightCanary` 精确切分。
金丝雀 Pod 与稳定版共用 ConfigMap、Secret 与同一个数据库，**新版本必须向后兼容现有 schema 与 Kafka 消息格式**。

```bash
helm upgrade --install couponkill charts/couponkill -n couponkill \
  -f charts/couponkill/values-prod.yaml -f charts/couponkill/values.canary-keda.yaml \
  --set-string image.tag=<稳定版> --set-string canary.coupon.tag=<新构建> --set-string canary.order.tag=<新构建>
```

验证通过后：`image.tag` 改成新构建号并去掉 `values.canary-keda.yaml` 即全量；回滚：去掉金丝雀 values 即可。

## KEDA

`keda.enabled=true`：order 的 `ScaledObject`（`so-<order 名>`）按消费组 `seckill-order-create-group` 在主题 `seckill_order_create` 上的积压扩缩。
消费组与主题名是 `internal/contracts/events` 里的协议常量，不要在 values 里改。KEDA 接管后 order 的 Deployment 不写 `replicas`、也不渲染 HPA。

## Kafka 主题

`kafka-topic-init`（post-install / post-upgrade Hook）只创建 `seckill_order_create`（分区数 `kafka.topics.seckillOrderCreate.partitions`）与
`seckill_order_create_dlt`（`kafka.topics.deadLetter.partitions`），已存在则只增加分区（Kafka 不允许减少）。
自带 Kafka 默认关闭自动建主题。外部 Kafka（`kafka.enabled=false` + `kafka.brokers`）同样由这个 Job 建主题。

## 监控

`monitoring.enabled=true`：Pod 带 `prometheus.io/scrape|port|path` 注解（路径 `/metrics`），并渲染一个 `ServiceMonitor`（需要 prometheus-operator CRD），
按 `app.kubernetes.io/part-of=couponkill` + `couponkill.io/scrape=true` 选择五个服务的 Service。`/metrics` 只在集群内：Istio 入口不放行，NetworkPolicy 只放行 `monitoring` 命名空间。

## 升级与运维注意

* **CK_WORKER_ID**：snowflake 节点号默认按主机名哈希（`-1`）推导，K8s 里 Pod 名随机，同一服务的副本之间有碰撞概率（0–1023 共 1024 个槽，碰撞表现为主键冲突）。
  不同服务写不同的表，所以只需要同一服务的副本互不相同；单副本服务可以用 `services.<svc>.extraEnv` 固定一个值。
  Deployment 的多副本无法在 chart 里分配不重叠的值——需要应用侧给出确定性方案（见 user/README 对 `CK_WORKER_ID` 的说明）。
* **Redis 持久化**：秒杀库存计数器 `ck:stock:*` 只存在于 Redis；自带 Redis 默认 AOF + `noeviction`。Redis 数据丢失后秒杀券处于「未预热」状态，需要 coupon 服务重启或管理员预热。
* **自带依赖是单节点**：PostgreSQL、Redis、Kafka 都是单副本 StatefulSet，适合开发与小规模；生产高可用请使用托管实例（`*.enabled=false`）并用 `networkPolicy.extraEgress` 放行出站。
* **从 Java 版升级**：这是一次不兼容迁移（Service 名未变，但 Secret 键名、环境变量、数据库 schema 都变了）。旧的 Nacos、Sentinel、`db-init`、`redis-init`、`nacos-config-watcher` 等对象
  不会被 Helm 自动清理时，请手动 `kubectl delete`。旧的 `couponkill-gateway-service` ServiceAccount 与 `seckill-go` 工作负载同理。
* 命名空间不再由 chart 创建（避免与 `--create-namespace` 冲突）；Istio 需要的 `istio-injection=enabled` 标签请自行打。

## 测试

渲染断言在独立的 Go 模块里，不连集群：

```bash
helm lint charts/couponkill
cd charts/couponkill/tests/render && go test ./...      # 需要 PATH 上有 helm；脚本行为测试需要 bash
```

覆盖：11 组 values 组合都能渲染且无重复对象、镜像引用合法；探针/安全上下文/资源；每个服务的 `CK_*` 与 Secret 键（含对照
`internal/platform/config`）；Secret 引用能解析；HPA/PDB/KEDA/金丝雀/Istio（只暴露 gateway、一个 host 一条 DestinationRule、权重合计 100）；
Kafka 初始化脚本行为（用假的 `kafka-topics.sh`）；`k8s-istio/` 授权策略与 chart 的 ServiceAccount、Deployment 标签一致；渲染期校验的失败用例。
CI（`.github/workflows/ci.yml` 的 `helm-chart` 作业）会跑这些测试。
