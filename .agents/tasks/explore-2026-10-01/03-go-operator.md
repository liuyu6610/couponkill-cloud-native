# 03 · go-operator 分片报告（couponkill-operator）

> 范围：仓库里的 Go 控制器模块 `couponkill-operator/`（`main.go`、`api/v1`、`controllers`、`config/`、`sample/`、`Dockerfile`、`README.md`）。秒杀运行时是另一个模块 `couponkill-go-service/`，本分片只在核对示例 CR 的环境变量时读它的配置结构，不重写 Go 服务本身。
> 方式：只读。未执行 `go test`、`controller-gen`、`make`、`kubectl`。基线：`main` @ `3969213`。
> 标注：【已核实】= 读代码或清单直接可见；【推断】= 按 controller-runtime / apiserver 语义推理，未在集群验证。
> 部署侧已有结论只留一行：Jenkins 在本目录跑 `make generate` 但这里没有 Makefile（05 R05）；Chart 探针参数渲染成 `:"8084"`（05 R25）；Chart CRD 与 `config/crd/bases` 不是同一 schema（05 R26）。

## 0. TL;DR

- 这块是什么：kubebuilder 风格的控制器，GVK 为 `ops.couponkill.io/v1` `Seckill`。Reconcile 按 spec 创建五个短名服务（go、coupon、order、user、gateway）的 Deployment、ClusterIP Service，以及可选的 CPU HPA，再把就绪情况写回 `status.phase`。
- 整体健康度：状态机的相位计算有单测，资源创建路径没有。控制器只负责「没有就创建」。README 里的 KEDA、Prometheus、Grafana、以及示例里的 `SeckillScalePolicy`，代码里都不存在。示例 CR 的镜像名和环境变量对不上 Java 服务和 `couponkill-go-service` 的真实配置方式。
- 最重要的 5 个发现：
  1. 【高·调和】`reconcileService` / `reconcileHPA` 只在 `IsNotFound` 时 `Create`。对象已经存在时直接返回，不比较 image、replicas、env、端口（`controllers/seckill_controller.go:152-163,258-269`）。把 `enabled` 改成 false 也不会删除已创建的 Deployment/Service/HPA（`:59-91,214-216`）。
  2. 【高·选择器】Pod 标签和 Service selector 只有 `app: <短名>`，例如 `app=coupon`（`:121-128,172-174`）。同一命名空间里两个 `Seckill` 会选中对方的 Pod。Helm 用的是 `app: couponkill-coupon-service` 这种全名（`charts/couponkill/values.yaml:95` 配 `deploy-coupon.yaml:23`），所以不会被这套短标签抢走，两套部署是平行的，也互不接管。
  3. 【高·契约】示例 `sample/ops_v1_seckill.yaml` 使用 `coupon:latest`、`go:latest` 这类镜像；Java 环境变量是 `SPRING_DATASOURCE_*` 和 `SPRING_REDIS_HOST`；Go 环境变量是 `POSTGRES_DSN`、`REDIS_ADDR`。Java 业务服务的数据源来自 Nacos 里的 ShardingSphere YAML，不读 `spring.datasource`（user 侧见 01 的 `UserShardingSphereConfig`）。`couponkill-go-service/internal/config/config.go` 的结构体没有这些 env，进程只读 Nacos/本地 yaml。按这份示例创建出来的 Pod 不是这套系统的进程【推断：镜像名与官方库镜像或空仓库名冲突，且 env 没有读者】。
  4. 【高·空实现】`Scaling.KEDA`、`Monitoring` 在类型和示例里都有，`Reconcile` 不读它们（`seckill_types.go:127-185`，`seckill_controller.go:93-98` 只处理 HPA）。`sample/seckillscalepolicy.yaml` 的 kind `SeckillScalePolicy` 在 `api/v1` 没有 Go 类型，`config/crd/bases` 里也没有第二份 CRD。spec 没有 connector。README 第 2、3 节宣称的 KEDA 和 Grafana 集成没有对应代码。
  5. 【中·健壮性】每次成功调和都 `Status().Update`（`:363`），主资源没有 Generation 谓词。HPA 的 `averageUtilization` 直接传递可能为 nil 的指针（`:245`）。`resource.MustParse` 遇到非法 quantity 会 panic（`:401`）。子 Deployment 没有探针、没有安全上下文。Chart 安装路径的探针引号、CRD 分叉、Jenkins `make` 见 05 R25 / R26 / R05。

## 1. 范围与技术栈

| 项 | 版本 / 事实 | 证据 |
|---|---|---|
| 模块路径 | `couponkill-operator`（`go.mod` 的 module 行）。仓库根没有 `go.mod` | `couponkill-operator/go.mod:1` |
| Go | `go 1.23.0`，toolchain `go1.23.11` | `go.mod:3-5` |
| 控制器 | `sigs.k8s.io/controller-runtime` v0.17.0；client-go / api / apimachinery v0.29.0 | `go.mod:10-13` |
| CRD 生成器 | `controller-gen.kubebuilder.io/version: v0.14.0` | `config/crd/bases/ops.couponkill.io_seckills.yaml:5` |
| 镜像 | 构建 `golang:1.23-alpine`，`GOARCH=amd64`；运行 `gcr.io/distroless/static:nonroot`。构建上下文是仓库根（COPY 路径带 `couponkill-operator/` 前缀） | `couponkill-operator/Dockerfile:2,13-16,20,24` |
| 安装清单 | kustomize：`config/crd`、`config/rbac`、`config/manager`、`config/default`、`config/prometheus`。另外 Chart 有自己的 operator 模板（05，不重复） | 目录清单 |
| 测试 | `controllers/seckill_status_test.go` 用 fake client 测 `updateStatus`。`suite_test.go` 启动 envtest，但没有任何 `It(...)` 用例 | 两个测试文件 |
| 本目录没有 | Makefile、`project/` 脚手架文件、connector 类型、KEDA 类型、`SeckillScalePolicy` 类型 | 目录与 `api/v1` 检索 |

05 报告写 CI 不跑 operator 测试（R38）。本分片未执行 `go test`。

## 2. 结构地图

```
couponkill-operator/
  main.go                         manager：metrics :8080，探针 :8081，leader-elect 默认 false
  api/v1/groupversion_info.go     Group ops.couponkill.io / v1
  api/v1/seckill_types.go         Seckill spec/status
  api/v1/zz_generated.deepcopy.go
  controllers/seckill_controller.go
  controllers/seckill_status_test.go
  controllers/suite_test.go       envtest 外壳，无规格
  config/crd/bases/ops.couponkill.io_seckills.yaml
  config/rbac/role.yaml           manager-role：Deployment/Service/HPA/Seckill
  config/rbac/leader_election_role.yaml
  config/manager/manager.yaml     Deployment，args 含 --leader-elect，探针 :8081
  config/prometheus/monitor.yaml
  sample/ops_v1_seckill.yaml      示例 Seckill（default 命名空间）
  sample/seckillscalepolicy.yaml  无对应类型的示例
  Dockerfile
  README.md                       宣称 KEDA 与 Grafana
```

`main.go` 的 flag 默认值：`--metrics-bind-address=:8080`，`--health-probe-bind-address=:8081`，`--leader-elect=false`（`main.go:41-45`）。kustomize 的 manager 清单改成了 `--leader-elect` 且探针仍是 `:8081`（`config/manager/manager.yaml:71-73,81-90`）。Chart 把探针绑到 8084 并且多打了一层引号，那是 05 R25，这里不展开。metrics 端口没有对应的 flag 注入，`METRICS_PORT` 环境变量无人读取这一条也已经在 05。

Leader election id 写死为 `80807133.couponkill.io`（`main.go:58`）。RBAC marker 没有 leases；选举用的 Role 在 `config/rbac/leader_election_role.yaml`，要和 manager 的 `--leader-elect` 一起装上。只装 `role.yaml` 就打开选举，会在拿锁时被拒绝【推断】。

## 3. 核心流程

```
Seckill 事件
  -> Get；NotFound 则返回（依赖 ownerRef GC，没有 finalizer）
  -> 对 go/coupon/order/user/gateway：
        enabled == true  -> reconcileService
        enabled == false -> 跳过（已有对象保留）
  -> scaling.hpa.enabled == true -> 给每个仍 enabled 的服务建 HPA
  -> updateStatus -> Status().Update
  -> return Result{}（不周期 Requeue）
```

`reconcileService` 的对象名是 `{cr}-{短名}` 和 `{cr}-{短名}-svc`，命名空间跟 CR 走（`seckill_controller.go:115-116,168-169`）。Deployment 副本数直接用 spec 里的 `*int32`，调用方没给时是 nil，apiserver 会把副本数当成 1【推断：apps/v1 的默认值】。容器端口用的是 `ServicePort.Port`，不是 `TargetPort`（`convertPorts`，`:371-378`）。示例里两者相等，所以示例本身不会踩中；两者不同时，容器声明端口是 Service 端口【已核实代码】。

已存在分支（`:161-163` 和 HPA 的 `:267-269`）没有 `Update`、没有 patch。改 CR 的 image 或副本数不会滚动 Pod【已核实：没有更新调用】。Deployment 状态变化仍会入队，因为 `SetupWithManager` `Owns` Deployment/Service/HPA（`:440-445`），但入队之后还是走「已存在则返回」。

`updateStatus`（`:275-367`）只统计 `enabled` 的服务：

| 条件 | `status.phase` | 条件 Ready |
|---|---|---|
| 没有启用任何服务 | `Pending` | False / ServicesNotReady |
| Deployment 不存在，或就绪数 = 0 | 全体如此则 `Progressing` | False |
| 部分就绪 | `Degraded` | False |
| 每个启用服务的 `ReadyReplicas >= spec.replicas`（nil 当 1） | `Ready` | True / AllServicesReady |

`seckill_status_test.go` 覆盖了「两个 Deployment 都就绪 -> Ready」和「Deployment 缺失 -> Progressing」。没有覆盖创建、更新、选择器、HPA。

删除 CR 时，已设置 `SetControllerReference` 的对象会随 owner 删除【推断：无 finalizer，GC 靠 ownerRef；代码在 NotFound 时明确不做事，`:46-50`】。`enabled: false` 不是删除。

## 4. 对外契约

CR 组 `ops.couponkill.io`，版本 `v1`，命名空间级，status 子资源（`seckill_types.go:214-216`，`groupversion_info.go:10`）。

Spec 字段（生成 CRD 与 Go 类型一致；Chart 那份不一致，见 05 R26）：

| 字段 | 控制器是否使用 |
|---|---|
| `spec.services.{go,coupon,order,user,gateway}Service` | 仅 `enabled==true` 时创建 |
| `image` / `replicas` / `resources` / `ports` / `env` | 只在创建时写入 |
| `env[].valueFrom.configMapKeyRef` / `secretKeyRef` | 创建时转换（`:415-432`） |
| `spec.scaling.hpa` | `enabled` 时创建 HPA；`minReplicas` 可空；`maxReplicas` 为 0 值时会提交 0【推断：HPA 校验失败】 |
| `spec.scaling.hpa.targetCPUUtilizationPercentage` | 指针原样放进 `averageUtilization`；nil 会被 API 拒绝【推断】 |
| `spec.scaling.keda` | 不读 |
| `spec.monitoring` | 不读 |

示例 CR（`sample/ops_v1_seckill.yaml`）同时打开了 HPA、KEDA 和 monitoring，镜像与环境变量如下，均与运行中的服务配置方式不一致：

| 服务 | 示例镜像 | 示例端口 | 示例环境变量 | 代码实际要的 |
|---|---|---|---|---|
| go | `go:latest` | 8090 | `REDIS_ADDR`、`POSTGRES_DSN`（含口令） | Go 从 Nacos `go-service-dev.yaml` / 本地 `config.yaml` 读 `postgres` 与 `data.redis`；`os.Getenv` 只有 Nacos 地址和连接池上限（`config.go:173-178`，`postgres_client.go:30-33`）。不读 `POSTGRES_DSN` / `REDIS_ADDR`【已核实检索】 |
| coupon / order / user | `coupon:latest` 等 | 8081 / 8082 / 8083 | `SPRING_DATASOURCE_URL` 指向单个库，`SPRING_REDIS_HOST` | Boot 4 的 Redis 键是 `spring.data.redis.host`（对应 `SPRING_DATA_REDIS_HOST`）。数据源是 ShardingSphere YAML，不是 `spring.datasource`【已核实：01 的 user 配置；coupon/order 同模式见 02】 |
| gateway | `gateway:latest` | 8080 | 无 | 路由在 Nacos，不在容器 env |

示例把数据库口令写在 CR 明文 env 里。仓库里其他明文口令的清单是 05 R34，这里不抄值。

kustomize manager 的探针是 `/healthz` 与 `/readyz`，端口 8081，实现是 `healthz.Ping`（`main.go:80-86`）。这和业务 Pod 无关：控制器创建的业务 Deployment 没有 liveness/readiness。

`config/prometheus/monitor.yaml` 是给控制器自己准备的 ServiceMonitor 脚手架。业务监控字段不会变成 PodMonitor 或 Grafana。

## 5. 依赖关系

```
（人）kubectl apply -k config/default     或     Helm deploy-operator.yaml（05）
        -> manager Pod
              -> 监听 Seckill
              -> 创建 Deployment / Service / HPA
                    标签 app=go|coupon|order|user|gateway
Helm charts/couponkill
        -> 另一套 Deployment，标签 app=couponkill-*-service / seckill-go / couponkill-gateway
```

控制器 RBAC（`config/rbac/role.yaml`）允许 apps/deployments、core/services、autoscaling/horizontalpodautoscalers、ops.couponkill.io/seckills（含 status 和 finalizers）的读写。没有 configmaps、secrets、events、scaledobjects、servicemonitors。所以就算以后想调和 KEDA，当前 ClusterRole 也不够【已核实清单；运行期拒绝是推断】。

代码声明了 finalizers 的 RBAC，Reconcile 从不 `AddFinalizer`【已核实】。

和 Helm 的关系是两套安装器，不是「Operator 接管 Chart」。示例若应用到 `couponkill` 命名空间，会多出一套名为 `{cr}-coupon` 的 Deployment，标签 `app=coupon`，Service 也只选这套标签。Helm Service 选 `app=couponkill-coupon-service`。流量不会自动接到 Helm 的 Pod 上，也不会被 Helm 的 Service 选中【已核实：两边的 selector 字符串不同】。

## 6. 问题与风险

| 级别 | 类型 | 结论 | 证据 |
|---|---|---|---|
| 高 | 调和 | 创建后 spec 漂移不会被修正；关闭 enabled 留下孤儿 Deployment/Service/HPA | `seckill_controller.go:152-163,214-216,258-269` |
| 高 | 多租户 | 短标签 `app=coupon` 等在命名空间内全局。两个 CR，或一个 CR 与人手动创建的同标签 Pod，会串到同一个 Service | `:121-128,172-174` |
| 高 | 契约 | 示例镜像、端口、环境变量不能启动本仓库的 Java/Go 服务。KEDA/monitoring 打开了也没有对象被创建 | `sample/ops_v1_seckill.yaml`；`config.go` 无 `POSTGRES_DSN`；`Reconcile` 不读 KEDA |
| 高 | 文档 | README 把 KEDA、Grafana、配置热更新写成已实现能力 | `couponkill-operator/README.md:12-25` 对比控制器 |
| 中 | 正确性 | HPA `maxReplicas` 零值和 nil 的 CPU 目标会在创建时被 API 拒绝，Reconcile 返回错误后重试【推断】 | `:237-245` |
| 中 | 稳定 | 每次调和都写 status。若 apiserver 对相同 status 仍递增 resourceVersion，主资源 watch 会热循环【推断】。fake client 单测看不出这一点 | `:363`；`SetupWithManager` 无谓词，`:440-445` |
| 中 | 稳定 | `MustParse` panic。controller-runtime v0.17 是否默认 RecoverPanic，未在本仓库配置里看到显式开关【推断】 | `:401` |
| 中 | 安全 | 业务 Pod 无 `securityContext`、无非 root、无只读根文件系统。manager 清单本身有 `runAsNonRoot` 和 drop ALL（`manager.yaml:59-80`） | `reconcileService` 的 PodSpec 只有 Containers |
| 低 | 死示例 | `SeckillScalePolicy` 无法被这只 controller 接受 | `sample/seckillscalepolicy.yaml:2`；`api/v1` 无此类型 |
| 低 | 测试 | envtest 套件没有用例；CI 不跑本模块（05 R38） | `suite_test.go:50-80` 只有 Before/After |

Chart 路径另有：探针地址 `:"8084"`（R25）、两份 CRD（R26）、Jenkins `make generate` 失败（R05）、operator 镜像只构建 amd64 且长期未随业务改（05 的 Dockerfile 活跃度表）。那些不在这里重写。

`convertEnvVars` 在同时带 `value` 和 `valueFrom` 时两个都写上（`:410-432`）。Kubernetes 不允许二者并存，创建会被拒绝【推断】。示例的 `JWT_SECRET` 只用了 `secretKeyRef`，那一条本身合法。

## 7. 最近活跃度

目录上共 6 个提交。最近两次在基线当天，从提交说明看是 Postgres 命名和 Chart 集群键，不是控制器行为重写：

| 提交 | 日期 | 说明 |
|---|---|---|
| `0e10e22` | 2026-07-18 | postgresclient 重命名等（触及本目录） |
| `cb49c6d` | 2026-07-18 | Chart 以 postgres.cluster 为真源 |
| `163b157` | 2025-09-03 | 「细节补充完毕」 |
| `06641ac` | 2025-08-30 | 一键部署 |
| `49dd5b7` | 2025-08-28 | Helm 一键部署 |

控制器实现停留在 2025-08/09 的脚手架形态：创建资源、写状态，没有后续的调和与收缩。

## 8. 未验证 / 不确定项

- 没跑 `go test`，也没把 CR apply 到集群。Create-only、选择器冲突、KEDA 无对象，都是读 Reconcile 得到的。
- HPA 零值、`MustParse` panic、status 热循环、ownerRef GC、nil replicas 默认为 1，是 API 语义推断。
- 示例镜像 `go:latest` 在 Docker Hub 上是 Go 工具链镜像还是拉不下来，没有拉镜像。结论只依赖「这个名字不是本仓库的 seckill-go 镜像名」。
- 根目录 Makefile 的 `generate`/`manifests` 指向仓库根的 `./...`（05 已写）。本目录确认没有 Makefile。没有跑 `controller-gen` 去看生成物是否脏。

## 9. 候选 OpenSpec capability（只写现状，不建 spec）

- `seckill-platform`：用一个 CR 声明五个服务的副本和镜像。现状是只创建、不更新、不删除，选择器不含 CR 名。
- `seckill-autoscaling`：HPA 有创建路径；KEDA 和 `SeckillScalePolicy` 只有类型或示例，没有调和。
- `seckill-observability`：status.phase 有实现和单测；Prometheus/Grafana 只有 README 和未使用的 spec 字段。
