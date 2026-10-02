# 05 deploy-cicd 分片探查报告

> 范围：charts/couponkill、k8s-istio、k8s-nothing、cross-namespace-monitoring、nacos、docker-compose.migration.yml、7 个模块 Dockerfile、.dockerignore、Makefile、build.ps1、Jenkinsfile、.github/workflows/\*、docs/\*-SOURCE-OF-TRUTH.md
> 模式：OpenSpec explore（只读）。仓库 HEAD：`3969213`（main）。报告日期：2026-10-01。
> 口径：路径均相对仓库根目录。「已核实」= 读过文件或跑过命令；「推断」= 基于代码与工具已知语义的推理，没有在集群或流水线上执行。敏感值一律只写 key 名。

## 0. TL;DR

- 这块是什么：CouponKill 的交付面。Helm Chart `charts/couponkill` 是文档声明的唯一生产真源；`docker-compose.migration.yml` + `nacos/` + `scripts/import-nacos-local.ps1` 是本地联调链路；`k8s-istio/` 是 Istio 叠加样例；`k8s-nothing/`、`cross-namespace-monitoring/` 已废弃；CD 用 Jenkins，CI 用 GitHub Actions（外加 Qodana），Makefile/build.ps1 是本地构建入口。
- 整体健康度：本地 compose 链路是自洽的（`docker compose config` 通过）。Helm/Jenkins 这条"生产真源"链路 `helm lint`、`helm template` 都能过，但渲染出来的东西和代码契约多处对不上，按现状推断装不起来、也跑不通。一句话：文档已经收敛了，制品还没有。
- 最重要的 5 个发现：
  1. 构建和 CD 链路断了。5 个 Java Dockerfile 的 builder 镜像 `maven:3.9.9-eclipse-temurin-25` 在 Docker Hub 不存在（已核实，3.9.9 最高只有 temurin-24）。Jenkins 在没有 Makefile 的 `couponkill-operator/` 里跑 `make generate`。Jenkins 推的是 `.../my-docker/<svc>:${BUILD_NUMBER}`，Chart 拉的却是 `.../my-docker:<svc>`，而且模板根本不读 `image.tag`，所以 `--set image.tag=${BUILD_NUMBER}` 什么都不做。
  2. Helm 安装本身大概率失败。三种 values 组合都会在同一个 release 里渲染出两个 `Service nacos`；Chart 自带 `Namespace couponkill`，和所有部署命令里的 `--create-namespace` 冲突；`kafka-init` 这个 post-install hook 在 apache/kafka 镜像里调用不存在的 `kafka-topics`，还在 `/bin/sh` 下用了 bash 语法，会一直循环到 hook 超时。
  3. 秒杀热路径 coupon 在 Chart 里基本不可用。镜像被渲染成 `my-docker:coupon}`（多了一个 `}`，已核实）；没有 Service，也没有 HPA 模板；values-prod 把端口定为 8081，但应用实际监听 8080（Chart 没给 coupon 注入 SERVER_PORT）。
  4. K8s 里的 Nacos 既连不上，内容也不全。Service 只暴露 8848，缺少 nacos-client 3.x 必需的 gRPC 端口 9848；nacos-init 默认去拉一个占位 URL，fallback 分支只往 public 命名空间写一份内联副本。应用读的是命名空间 120/998 里的 `couponkill-<svc>.yaml`、`gateway-routes.yaml`、`*-service-sharding.yaml`，Helm 链路一个都不提供。Nacos 配置实际有三套互相分叉的副本（`nacos/`、nacos-init 内联、values-prod 里的死内容）。
  5. 安全问题。Istio 入口的 VirtualService 直接路由到 user/order/coupon，绕开了负责 JWT 校验的 Spring Gateway，而下游只认请求头 `X-User-Id`/`X-User-Roles`，所以身份和 admin 角色都能伪造。`/nacos`、`/sentinel` 经入口对外公开，prod 还给 Nacos 开了 LoadBalancer，而 Nacos 里存着明文 DB 密码和 JWT secret。`k8s-nothing/couponkill-simple/couponKill.yaml` 提交了 dockerconfigjson 镜像仓库凭据。NetworkPolicy 默认拒绝全部出站，却没有任何放行规则，连 DNS 都被挡住。
- 建议修复顺序：先修构建（R01、R05），再让 Chart 能装上（R02、R06、R08、R09），接着打通 Nacos（R10 至 R13），最后处理安全（R16 至 R18）。编号对应第 6 节。

## 1. 范围与技术栈

### 1.1 范围

- 深读：`charts/couponkill/`（Chart.yaml、3 份 values、27 个模板、scripts/\*.sql、README）、`k8s-istio/`、`k8s-nothing/couponkill-simple/`、`cross-namespace-monitoring/`、`nacos/`（21 个文件）、`docker-compose.migration.yml`、`couponkill-*/Dockerfile`（7 个）、`.dockerignore`、`Makefile`、`build.ps1`、`Jenkinsfile`、`.github/workflows/ci.yml`、`.github/workflows/qodana_code_quality.yml`、`qodana.yaml`、`docs/CICD-SOURCE-OF-TRUTH.md`、`docs/DEPLOYMENT-SOURCE-OF-TRUTH.md`。
- 只读到能核对接口为止：各服务 `src/main/resources/application.yml`、`scripts/import-nacos-local.ps1`、Go 的 `internal/config/config.go`、operator 的 `main.go` 与 `config/crd/bases`、几个 Java 类（UserContextFilter、JwtAuthGlobalFilter、ConnectorAdminInterceptor、\*ShardingSphereConfig、Kafka listener）。
- 只定位、不深读：`ansible/`、`couponkill-operator/` 的控制器逻辑。

### 1.2 技术栈与版本（已核实）

| 类别 | 选型 / 版本 | 证据 |
|---|---|---|
| Helm Chart | apiVersion v2，chart 1.0.0，appVersion "2.0.0"；无子 chart，无 `dependencies:` | charts/couponkill/Chart.yaml:3-5,20 |
| 集群前提（README 声称） | Kubernetes 1.27.0+、Helm 3、Istio 1.18.0、KEDA 2.17.0 | charts/couponkill/README.md「前提条件」 |
| Java 平台 | JDK 25；Spring Boot 4.0.5；Spring Cloud 2025.1.0；Spring Cloud Alibaba 2025.1.0.0；nacos 3.1.1；PostgreSQL JDBC 42.7.4 | pom.xml:55-57,62-64,67,70 |
| Java 镜像 | builder `maven:3.9.9-eclipse-temurin-25`（Docker Hub 不存在）；运行时 `eclipse-temurin:25-jre-alpine`（存在） | couponkill-gateway/Dockerfile:2,9；couponkill-{user,coupon,connector}-service/Dockerfile:2,16；couponkill-order-service/Dockerfile:2,17 |
| Go 服务 | go.mod `go 1.23.0` / toolchain go1.23.11；镜像 `golang:1.23-alpine` -> `alpine:latest`；CI 用 Go 1.24.x | couponkill-go-service/go.mod:3,5；couponkill-go-service/Dockerfile:2,23；.github/workflows/ci.yml:39 |
| Operator | go 1.23；`golang:1.23-alpine` -> `gcr.io/distroless/static:nonroot`，只构建 amd64；controller-tools v0.14.0、envtest K8s 1.29.0（根 Makefile） | couponkill-operator/go.mod:3,5；couponkill-operator/Dockerfile:2,20,24,32；Makefile:4,253 |
| 中间件镜像 | postgres:16；redis:7（compose）、redis:latest（values.yaml，无模板引用）；apache/kafka:3.8.0；nacos/nacos-server:v3.1.1；curlimages/curl:latest（nacos-init 与 watcher）；Sentinel dashboard 1.8.5（values-prod），而镜像同步脚本推的是 bladex/sentinel-dashboard:1.8.6 | docker-compose.migration.yml:13,33,40,85；charts/couponkill/values.yaml:200-201,233,300,343-344；charts/couponkill/values-prod.yaml:448-449；Makefile 与 Jenkinsfile 的依赖镜像段 |
| CI | actions/checkout@v4、setup-java@v4（temurin 25）、setup-go@v5（1.24.x）；Qodana `JetBrains/qodana-action@v2025.2`，linter `qodana-jvm-community:2025.2`，`projectJDK: "21"` | .github/workflows/ci.yml:16-43；.github/workflows/qodana_code_quality.yml:29；qodana.yaml:26,49 |
| CD | Jenkins declarative，`agent any`；凭据 `docker-registry`、`kubeconfig`；Slack 通知 | Jenkinsfile:3-4,83,139-141,160-163 |
| 镜像仓库 | 阿里云个人版 ACR：`crpi-...personal.cr.aliyuncs.com/thetestspacefordocker/my-docker`（稳定）与 `.../canary-keda-dev`（金丝雀） | Jenkinsfile:7-8；Makefile:20-21；build.ps1:8-9；charts/couponkill/values.yaml:54 |

### 1.3 本次执行的校验

所有命令都是只读的，产物写在 `%TEMP%`，结束后已删除。

| 命令 | 结果 |
|---|---|
| `helm lint charts/couponkill`（默认、`-f values-prod.yaml`、`-f values.canary-keda.yaml` 各一次） | 3 次都是 `1 chart(s) linted, 0 chart(s) failed`，只有 INFO `icon is recommended` |
| `helm template couponkill charts/couponkill > %TEMP%\couponkill-rendered.yaml`，另外渲染了 prod 和 canary 两份 | 全部成功；资源数：默认 45，prod 71，canary 64 |
| 在渲染结果里查同名对象 | 默认：`Service/nacos` x2。prod：`Service/nacos` x2、`Service/sentinel-dashboard` x2、`VirtualService/couponkill-vs` x2。canary：`Service/nacos` x2、`VirtualService/couponkill-vs` x2 |
| `--set services.connector.enabled=false` | connector 的 Deployment 和 Service 照样渲染出来 |
| `--set services.connector.internalTokenSecret.optional=false` | 渲染结果仍是 `optional: true` |
| `--set services.gateway.hpa.enabled=true`（其余用 values.yaml 默认值） | 渲染出空的 `averageUtilization:` |
| `--set sentinel.enabled=true`（其余用默认值） | 模板报错：`sentinel.yaml:40:28 ... nil pointer evaluating interface {}.repository` |
| `--set nacos.cluster.enabled=true` | 只剩没有 selector 的 headless `nacos` 和有 selector 的 `nacos-headless`；应用连的 `nacos:8848` 没有后端 |
| `docker compose -f docker-compose.migration.yml config --quiet` | exit 0；服务为 postgres、redis、kafka、kafka-init、nacos |
| Docker Hub API：`library/maven` 中名字含 `3.9.9-eclipse-temurin` 的 tag | 共 26 个，JDK 最高到 `-24`，没有 `-25`；`3.9.11-eclipse-temurin-25` 存在 |
| OpenJDK jdk25u `arguments.cpp` 的特殊 flag 表 | `ZGenerational`：23 deprecated、24 obsolete、expire 未定义。所以在 JDK 25 上只打警告，不会阻止启动 |

外部来源：[Docker Hub maven tag 查询](https://hub.docker.com/v2/repositories/library/maven/tags?page_size=100&name=3.9.9-eclipse-temurin)、[maven:3.9.11-eclipse-temurin-25](https://hub.docker.com/v2/repositories/library/maven/tags/3.9.11-eclipse-temurin-25)、[jdk25u arguments.cpp](https://raw.githubusercontent.com/openjdk/jdk25u/master/src/hotspot/share/runtime/arguments.cpp)、[JEP 490](https://openjdk.org/jeps/490)（内容已改写转述）。

没有执行：`kubectl apply`、`helm install`、`docker build/run`、mvn、go、npm、Jenkins，也没有查看 GitHub Actions 的历史运行记录。

## 2. 结构地图

### 2.1 目录 -> 职责

| 路径 | 职责 | 文档定位 / 备注 |
|---|---|---|
| charts/couponkill/values.yaml | 默认（演示）values | 生产真源的一部分 |
| charts/couponkill/values-prod.yaml | 生产覆盖：副本数、HPA、Istio、Sentinel、KEDA、NetworkPolicy，外加一大段 Nacos 配置正文 | 大量键没有任何模板引用（见 4.9） |
| charts/couponkill/values.canary-keda.yaml | 金丝雀权重、KEDA Kafka lag、canary 镜像仓库 | 同上 |
| charts/couponkill/templates/ | 27 个模板（见 2.2） | |
| charts/couponkill/scripts/init-postgres.sql | 6 个分片库与分表 DDL，包含预约表、通知表 | 文档称 Schema 真源 |
| charts/couponkill/scripts/02-seed-demo.sql | 演示种子数据（user_db_0、coupon_db_0） | 只有 compose 会执行 |
| charts/couponkill/scripts/03-init-connector.sql | 建 connector_db，以及 platform_sku_binding、coupon_price_map | 只有 compose 会执行 |
| charts/couponkill/scripts/04-seckill-reservation.sql | 存量库迁移：给 coupon_\* 补 seckill_start_at/end_at，给 order_db_0 补预约表和通知表 | 没有任何入口会执行；内容已并入 init-postgres.sql |
| charts/couponkill/scripts/init.sql.deprecated-mysql | MySQL 时代的单库 DDL（user、coupon、order、user_coupon，InnoDB） | 死文件，无引用 |
| nacos/ | Nacos 配置的仓库副本：DEFAULT_GROUP 16 个、shard/DEFAULT_GROUP 3 个、KAFKA_GROUP 1 个、SENTINEL_GROUP 1 个 | 本地导入源；Chart 不读取 |
| scripts/import-nacos-local.ps1（ops 分片） | 把 nacos/ 导入本地 Nacos（命名空间 120/998），并把主机名本地化 | 只读，没有执行 |
| docker-compose.migration.yml | 本地 PG、Redis、Kafka、Nacos | 本地联调，非生产 |
| k8s-istio/ | 10 份 Istio 清单：Namespace、Gateway、VS、DR、AuthorizationPolicy、PeerAuthentication、ServiceEntry、Sidecar、Telemetry、EnvoyFilter | 叠加样例 |
| k8s-nothing/couponkill-simple/ | MySQL + RocketMQ 时代的多容器裸 Pod 一体包 | DEPRECATED |
| cross-namespace-monitoring/ | monitoring 命名空间下 5 个 ServiceMonitor、RBAC、示例 values | DEPRECATED |
| couponkill-\*/Dockerfile | 7 个镜像的构建文件 | |
| .dockerignore | 只忽略 `.git` 和 `.gitignore` | |
| Makefile | kubebuilder 脚手架（operator）加项目镜像与 Helm 目标 | 本地构建入口 |
| build.ps1 | Makefile 项目目标的 Windows 版本 | 本地构建入口 |
| Jenkinsfile | 构建 -> 镜像 -> 推送 -> helm 部署（主 release 和 canary） | CD 真源 |
| .github/workflows/ci.yml | PR 与 push 时跑单测 | CI 真源 |
| .github/workflows/qodana_code_quality.yml、qodana.yaml | 可选的质量扫描 | |
| docs/CICD-SOURCE-OF-TRUTH.md、docs/DEPLOYMENT-SOURCE-OF-TRUTH.md | 真源声明，2026-07-18 生效 | |
| ansible/（未深读） | 历史 ECS 剧本，包含 `roles/deploy_service/templates/docker-compose.yml.j2` | DEPRECATED |
| couponkill-operator/（未深读） | Seckill CRD 和控制器，另有 kustomize 的 `config/{crd,default,manager}` | 可选 |

### 2.2 Chart 模板 -> 渲染出的资源

| 模板 | 渲染条件 | 资源 |
|---|---|---|
| couponConfig.yaml | 总是 | Namespace `{{namespace}}`（label `istio-injection: enabled`）、Namespace `{{namespaceMonitor}}`（monitor） |
| crd.yaml | `crd.install`（默认 true） | CRD `seckills.ops.couponkill.io` |
| deploy-serviceAccount.yaml | 总是 | 5 个 ServiceAccount（serviceAccount0 至 4） |
| deploy-gateway.yaml | 总是 | Deployment + Service（80 -> 8080）+ HPA（hpa.enabled 时） |
| deploy-user.yaml | 总是 | Deployment + Service + HPA（可选） |
| deploy-coupon.yaml | 总是 | 只有 Deployment，没有 Service，也没有 HPA |
| deploy-order.yaml | 总是 | Deployment + Service（端口没有名字）+ HPA（可选） |
| deploy-connector.yaml | `services.connector.enabled \| default true`，实际上关不掉 | Deployment + Service（没有 ServiceAccount） |
| deploy-go.yaml | 总是 | Deployment + Service + HPA（可选） |
| deploy-operator.yaml | 总是 | Deployment + Service + SA + Role/RoleBinding + ClusterRole/ClusterRoleBinding |
| dependencies.yaml | `dependencies.enabled`（postgres、redis、broker）；`nacos.enabled`（nacos）；`sentinel.enabled` | 没有 selector 的 headless Service：postgres、postgres-master、postgres-slave、redis、redis-master、redis-slave、broker（没有 Kafka STS 时）、nacos、sentinel-dashboard |
| kafka-statefulset.yaml | `kafka.enabled && kafka.statefulSet.enabled`（默认 true） | Service kafka-headless、kafka、broker + StatefulSet kafka（单副本 KRaft，10Gi PVC） |
| kafka-init-job.yaml | `kafka.init.enabled`（默认 true） | Job（post-install/post-upgrade hook） |
| db-init-job.yaml | `db.init.enabled && dependencies.enabled`（默认 false，prod 为 true） | Job（hook）+ ConfigMap db-init-sql |
| redis-init-job.yaml | `redis.init.enabled`（只有 prod 有） | Job（hook） |
| nacos.yaml | `nacos.enabled`；按 `cluster.enabled` 二选一 | 单机：Service nacos（ClusterIP）+ nacos-external（可选）+ Deployment。集群：ConfigMap + Service nacos-headless + StatefulSet |
| nacos-init-job.yaml | `nacos.enabled` | Job（hook） |
| nacos-config-watcher.yaml | `nacos.enabled && configWatcher.enabled`（默认 true） | CronJob，每 5 分钟一次 |
| sentinel.yaml | `sentinel.enabled`（prod） | dashboard 的 Service + Deployment；tokenServer 的 Service + Deployment |
| istio.yaml | `istio.enabled`（prod、canary） | Gateway + VirtualService couponkill-vs + 7 个 DestinationRule |
| destinationrules.yaml | `istio.enabled` | DR dr-couponkill-coupon-service、dr-couponkill-order-service（子集 stable/canary） |
| virtualservice-canary.yaml | `istio.enabled` | VirtualService couponkill-vs（和 istio.yaml 里的同名） |
| keda.yaml | `keda.enabled && keda.kafka` | ScaledObject（goEdge、order 各自有开关）+ TriggerAuthentication |
| monitoring.yaml | `monitoring.enabled && monitoring.prometheus.enabled`（只有 canary 满足） | 4 个 ServiceMonitor + 1 个 PodMonitor |
| network.yaml | 总是 | NetworkPolicy default-deny、allow-istio-system、allow-same-namespace |
| configmap.yaml | `.Values.configmap.enabled`（三份 values 里都没有这个键） | 永远不会渲染 |
| seckill-example.yaml | `examples.seckill`（默认 false） | Seckill CR 示例 |

Chart 不会渲染的东西：PostgreSQL 和 Redis 的工作负载、Secret、PodDisruptionBudget、Ingress、PeerAuthentication、PrometheusRule。模板里 grep 不到 affinity、topologySpreadConstraints、imagePullSecrets。

## 3. 核心流程

### 3.1 CI/CD 链路（文档语义与实际对接点）

```
 developer
    |  PR / push (develop | main)
    v
 +---------------------------------------------------------------+
 | GitHub Actions  .github/workflows/ci.yml                       |
 |   java-test: temurin 25                                        |
 |     mvn -B -pl coupon,order,user,gateway -am test              |
 |         -Dtest='!*ApplicationTests'                  (:27,:29) |
 |   go-test:   go 1.24.x, couponkill-go-service go test ./...    |
 |   (no connector, no operator, no docker build, no helm lint)   |
 +---------------------------------------------------------------+
    |  PR (any branch) / push (dev | main)
    v
 +---------------------------------------------------------------+
 | Qodana  .github/workflows/qodana_code_quality.yml              |
 |   qodana-jvm-community:2025.2, projectJDK "21"  (qodana.yaml)  |
 +---------------------------------------------------------------+

 Jenkins (manual / scheduled)                        Jenkinsfile
    |
    v
 [Build, parallel]
    mvn clean package -DskipTests                           (:18)
    go build -o seckill-go ./cmd/server                     (:23)
    cd couponkill-operator && make generate && make manifests (:28)
        x  no Makefile in couponkill-operator/
    |
    v
 [Build Docker Images, parallel: 7 services x 2 builds]
    ${REGISTRY}/<svc>:${BUILD_NUMBER}                       (:38)
    ${CANARY_REGISTRY}/<svc>:canary                         (:39)
        x  Java Dockerfiles: FROM maven:3.9.9-eclipse-temurin-25 (tag missing)
    |
    v
 [Push]         docker push ${REGISTRY}/<svc>:${BUILD_NUMBER}   (:85)
 [Mirror deps]  postgres, redis, nacos-server, sentinel-dashboard, kafka
                -> ${REGISTRY}/<name>  (path style)             (:112)
    |
    v
 [Deploy]  helm upgrade --install couponkill ./charts/couponkill
             --namespace couponkill --create-namespace
             --set image.tag=${BUILD_NUMBER}                    (:141)
           templates render "<image.registry>:<services.X.image.name>"
           -> pulls .../my-docker:gateway (tag style); image.tag never read
    |
    v
 [Deploy Canary]  helm upgrade --install couponkill-canary ...
                    -f values.canary-keda.yaml                  (:149)
           same Namespace / CRD / ClusterRole / Deployment names
           as release "couponkill"  x ownership conflict
    |
    v
 [post]  docker logout ; slackSend                        (:160, :163)
```

说明：CI 和 CD 之间没有产物传递（CD 不复用 CI 结果，也不跑测试）。CD 的"构建 -> 推送 -> 部署"三段各自有一个断点：基础镜像不存在、镜像命名风格不一致、canary release 冲突。所以文档说的"集群状态以 Helm release 为准"，在当前实现里拿不到一个由 Jenkins 构建号驱动的 release。

### 3.2 Helm 安装时序（按当前模板推断的失败点）

```
 helm upgrade --install couponkill charts/couponkill -n couponkill --create-namespace [-f values-prod.yaml]
    |
    | (1) Helm creates namespace "couponkill" itself (no Helm ownership labels)
    v
 (2) render 45 objects (default) / 71 (prod)
    |   x Namespace couponkill is also in the chart (couponConfig.yaml)
    |       -> "exists and cannot be imported: invalid ownership metadata"
    |   x Service nacos rendered twice (dependencies.yaml + nacos.yaml)
    |   x prod: Service sentinel-dashboard x2, VirtualService couponkill-vs x2
    v
 (3) create workloads
    |   gateway, user, coupon (image ".../my-docker:coupon}" -> InvalidImageName),
    |   order, connector, seckill-go, operator (probe addr :"8084")
    |   kafka StatefulSet (1 replica KRaft)
    |   nacos Deployment (standalone, emptyDir, port 8848 only, no 9848)
    |   postgres / redis / redis-master = headless Services WITHOUT selector
    |   NetworkPolicy default-deny Ingress+Egress, no egress allow rule
    v
 (4) post-install hooks: sorted by weight, then by name; each waited up to --timeout
    |   db-init    (prod) : until pg_isready -h postgres           -> no backend, loops
    |   kafka-init        : until kafka-topics --list              -> binary not in image, loops
    |   nacos-init        : curl placeholder zip (external.enabled=true by default)
    |                       or inline defaults -> namespace "public"
    |   redis-init (prod) : until redis-cli -h redis ping          -> no backend, loops
    v
 (5) first hook exceeds --timeout (default 5m) -> release FAILED; later hooks never run
```

说明：第 (2) 步的两类冲突和第 (4) 步的 hook 排序，是按 Helm 3 已知行为推断的（hook 按 weight 排序、同 weight 按名字排序、逐个等待完成）。所有 hook 都没设 weight，所以默认 values 下 `kafka-init` 排在 `nacos-init` 前面。这些没有在集群上执行过。

### 3.3 本地联调链路（目前唯一自洽的路径）

```
 docker compose -f docker-compose.migration.yml up -d
    |
    +--> postgres:16   host 5433 -> 5432
    |       /docker-entrypoint-initdb.d:
    |         01-init-postgres.sql, 02-seed-demo.sql, 03-init-connector.sql
    +--> redis:7       6379, appendonly yes
    +--> kafka 3.8.0   KRaft; in-network kafka:29092, host localhost:9092
    |       kafka-init: seckill_order_create, seckill_order_result,
    |                   seckill_compensation, order_created  (16 partitions, RF 1)
    +--> nacos v3.1.1  standalone (Derby), 8848 + 9848,
    |                  NACOS_AUTH_ENABLE=false + token / identity placeholders
    |
    v
 pwsh scripts/import-nacos-local.ps1              (Nacos http://127.0.0.1:8848)
    |  ensure namespaces 120 (couponkill-dev) and 998 (couponkill-shard)
    |  tenant 120, DEFAULT_GROUP : nacos/DEFAULT_GROUP/*.yaml
    |                              + couponkill-<svc>.yaml copied from *-dev.yaml
    |  tenant 120, SENTINEL_GROUP: couponkill-gateway-sentinel (json)
    |  tenant 998, DEFAULT_GROUP : nacos/shard/DEFAULT_GROUP/*-service-sharding.yaml
    |  Localize(): postgres:5432 -> 127.0.0.1:5433 ; redis(-master) -> 127.0.0.1 ;
    |              kafka:9092 -> 127.0.0.1:9092   ; nacos:8848 -> 127.0.0.1:8848
    v
 Java services on the host (scripts/local-http-smoke.ps1)
    NACOS_SERVER_ADDR default localhost:8848, NACOS_NAMESPACE default 120,
    shardingsphere.namespace 998
```

说明：这是唯一一条 Nacos 命名空间（120/998）、dataId 命名（`couponkill-<svc>.yaml`、`*-service-sharding.yaml`）与应用代码完全对得上的链路。依据：scripts/import-nacos-local.ps1:7-8,59-83；couponkill-user-service/src/main/resources/application.yml:10-25。

### 3.4 入口流量路径（istio.enabled=true，prod 默认开启）

```
 client
   |
   v
 istio-ingressgateway  (Gateway <istio.gateway.name>, host couponkill.example.com, HTTP 80)
   |
   |  VirtualService couponkill-vs   (templates/istio.yaml)
   +-- /seckill        --> seckill-go(-svc):80           retries 3, timeout 5s
   +-- /api/v1/user    --> couponkill-user-service:80    timeout 10s
   +-- /api/v1/order   --> couponkill-order-service:80   timeout 10s
   +-- /api/v1/coupon  --> couponkill-coupon-service:80  x Service not rendered
   +-- /nacos          --> nacos:8848       (config center: plaintext DB / JWT secrets)
   +-- /sentinel       --> sentinel-dashboard:8080 (prod)
   |
   |  VirtualService couponkill-vs   (templates/virtualservice-canary.yaml, SAME NAME)
   +-- /api/v1/seckill --> coupon subsets stable/canary  (no pod has version=stable|canary)
   +-- /api/v1/auth    --> user
   +-- /api/v1         --> order subsets stable/canary   (also catches /api/v1/user, /api/v1/coupon)
   +-- "seckill"       --> seckill-go    (prefix has no leading "/", never matches)

   couponkill-gateway (Spring Cloud Gateway + JwtAuthGlobalFilter) is on NONE of these routes.
   Downstream UserContextFilter trusts X-User-Id / X-User-Roles exactly as the caller sends them.
```

说明：Spring Gateway 只在"客户端直连 gateway Service"时生效（例如本地 8088）。一旦走 Istio 入口，JWT 校验就被整个绕过了。依据：charts/couponkill/templates/istio.yaml:47-138；couponkill-common/src/main/java/com/aliyun/seckill/common/filter/UserContextFilter.java:28-33；couponkill-gateway/src/main/java/com/aliyun/seckill/couponkillgateway/security/JwtAuthGlobalFilter.java:114-118。

## 4. 对外契约

### 4.1 端口矩阵（同一个服务在各处的端口）

| 服务 | Dockerfile EXPOSE | Nacos `server.port` | values.yaml | values-prod.yaml | Helm 是否钉死监听端口 | K8s Service | 探针 |
|---|---|---|---|---|---|---|---|
| gateway | 8080（couponkill-gateway/Dockerfile:12） | 8088（nacos/DEFAULT_GROUP/couponkill-gateway-dev.yaml:2） | 8080（values.yaml:161） | 8080 | 是，SERVER_PORT（deploy-gateway.yaml:57-58） | couponkill-gateway 80 -> 8080 | /actuator/health |
| user | 8083（Dockerfile:19） | 8081（couponkill-user-service-dev.yaml:2） | 8081（values.yaml:75） | 8083（values-prod.yaml:583） | 是，SERVER_PORT | 80 -> port | /actuator/health |
| coupon | 8081（Dockerfile:19） | 8080（couponkill-coupon-service-dev.yaml:2） | 8080（values.yaml:96） | 8081（values-prod.yaml:514） | 否 | 没有 Service | /actuator/health |
| order | 8082（Dockerfile:20） | 8082（couponkill-order-service-dev.yaml:2） | 8082（values.yaml:117） | 8082 | 否 | 80 -> 8082（端口无名字，deploy-order.yaml:80） | 只有 readiness |
| connector | 8085（Dockerfile:19） | 8085 | 8085（values.yaml:139） | 8085 | 是，SERVER_PORT | 80 -> 8085 | /actuator/health |
| seckill-go | 8090（couponkill-go-service/Dockerfile:44） | 8083（go-service-dev.yaml:2） | 8083（values.yaml:63） | 8083（values-prod.yaml:496） | 设了 PORT，但代码不读 | 80 -> 8083 | /health |
| operator | 无 | 无 | 8084（values.yaml:182） | 8084 | `--health-probe-bind-address=:"8084"` | 8084 | /healthz、/readyz |
| nacos | 无 | 无 | 8848 | 8848 | - | nacos 8848（两份，都不含 9848） | 无 |
| kafka | 无 | 无 | 9092 | 9092 | - | kafka、broker 9092；kafka-headless 9092/9093 | exec kafka-topics.sh / tcp 9092 |
| 参照：k8s-nothing、cross-namespace-monitoring | - | - | - | - | - | coupon 8081、user 8083、go 8090（沿用 Dockerfile） | - |

结论：Helm 里只有 gateway、user、connector 用 SERVER_PORT 钉死了监听端口。coupon 和 order 监听哪个端口，取决于能不能从 Nacos 读到 `couponkill-<svc>.yaml`。Helm 链路读不到（见 4.3），读不到就退回 Spring 默认的 8080。同一个服务在 4 到 5 个地方写了不同的端口。

### 4.2 Helm 注入的环境变量与实际消费方

| 工作负载 | 模板注入的变量 | 代码或配置实际读取什么 | 结论 |
|---|---|---|---|
| gateway | `NACOS_SERVER_ADDR`、`NACOS_NAMESPACE=public`（deploy-gateway.yaml:62-65） | application.yml:16-23 读的就是 `${NACOS_SERVER_ADDR}`、`${NACOS_NAMESPACE:120}` | 生效。但 public 命名空间里没有 `couponkill-gateway.yaml`，也没有 `gateway-routes.yaml` |
| user、order、connector | `SPRING_CLOUD_NACOS_SERVER_ADDR`（deploy-user.yaml:83-87、deploy-order.yaml:48-51、deploy-connector.yaml:48-51） | application.yml 显式写了 `config.server-addr` 和 `discovery.server-addr: ${NACOS_SERVER_ADDR:localhost:8848}`（如 user:16,19） | 推断无效：通用键只在具体键为空时才兜底，所以 Pod 内会连 localhost:8848 |
| coupon | `SPRING_CLOUD_NACOS_{CONFIG,DISCOVERY}_SERVER_ADDR`（deploy-coupon.yaml 的 env 段） | 同上 | 生效，因为环境变量优先级高于文件 |
| 除 gateway 外的 Java 服务 | 没有 `NACOS_NAMESPACE` | 默认 120（application.yml 的 namespace 行） | Helm 部署的 Nacos 里没有 120；gateway 在 public 里做服务发现，其他服务注册到 120，`lb://` 路由会找不到实例（推断） |
| gateway、user | `JWT_SECRET` 来自 Secret `jwt-secret`/`secret`（deploy-gateway.yaml:77-81、deploy-user.yaml:92-96） | `jwt.secret` | 生效，但 Secret 要手工创建（4.6） |
| gateway、user | `SPRING_REDIS_HOST/PORT`（deploy-gateway.yaml:67-71、deploy-user.yaml:79-82） | Boot 4.0.5 使用 `spring.data.redis.*`（pom.xml:62） | 无效。默认 values 下 gateway 渲染出来还是空值（`redis.host` 未定义） |
| user | `SPRING_DATASOURCE_*`，密码明文（deploy-user.yaml:73-78） | ShardingSphere 数据源由 UserShardingSphereConfig 从 Nacos 998 拉取 | 作用不明，交给 java 分片核实 |
| order | `DB_USER`、`DB_PASS`（明文）、`KAFKA`（deploy-order.yaml:31-36） | 在 couponkill-\*/src/main 和 nacos/ 里 grep 都没有命中；Kafka 地址来自 common.yaml:7 的 `${KAFKA_BOOTSTRAP_SERVERS:kafka:9092}` | 死配置 |
| order、coupon、go | `POSTGRES_/REDIS_/KAFKA_CLUSTER_*` | grep 0 命中（Java、Go、operator、nacos） | 死配置 |
| user | `CONFIG_REFRESH_*`、`CONFIG_SOURCE_*`、`FEATURE_TOGGLE` | grep 0 命中 | 死配置 |
| 所有 Java 服务 | `JAVA_TOOL_OPTIONS`、`THREAD_POOL_OPTS` | JVM 自动读 JAVA_TOOL_OPTIONS；Dockerfile ENTRYPOINT 展开 `$THREAD_POOL_OPTS` | 生效 |
| connector | `CONNECTOR_DB_HOST`、`CONNECTOR_INTERNAL_TOKEN(_STRICT)`、`SPRING_PROFILES_ACTIVE=prod` | connector application.yml:22,58-59 | 生效；但数据库 `connector_db` 要靠 03 脚本建，Helm 不执行 |
| seckill-go | `PORT`、`REDIS_ADDR`、`POSTGRES_DSN`（内联明文密码）、`KAFKA`、`NACOS_CONFIG_*`（deploy-go.yaml:68-91） | 只读 `NACOS_SERVER_ADDR`、`NACOS_ADDR`、`NACOS_NAMESPACE_ID`（config.go:173-178）以及 `GO_DB_MAX_*` | 除 `NACOS_ADDR` 外都无效 |
| operator | `METRICS_PORT` | main.go:41-42 只用命令行 flag | 无效 |

### 4.3 Nacos dataId / group / namespace 矩阵

| 仓库文件 | 本地导入（dataId / group / tenant） | 代码中的消费方 | Helm nacos-init（fallback 分支） | 备注 |
|---|---|---|---|---|
| nacos/DEFAULT_GROUP/common.yaml | common.yaml / DEFAULT_GROUP / 120 | 5 个 Java 服务的 `spring.config.import`（如 user application.yml:11） | 往 public 发一份内联版（nacos-init-job.yaml:77），内容不同：没有 spring.kafka、没有 threads.virtual | Redis 主机名为 `redis-master`（common.yaml:21） |
| gateway-routes.yaml | gateway-routes.yaml / DEFAULT_GROUP / 120 | gateway（couponkill-gateway application.yml:11） | 不发。发的是 `gateway.yaml`（:152），而且缺 connector 路由和 `/api/v1/auth/**` | |
| couponkill-gateway-dev.yaml | 同名，另存一份 `couponkill-gateway.yaml` / 120（import-nacos-local.ps1:66） | gateway 的 `${spring.application.name}.yaml`（application.yml:12） | 不发 | 包含 server.port 8088、明文 `jwt.secret`（:91） |
| couponkill-{user,coupon,order,connector}-service-dev.yaml | 同名，另存一份 `couponkill-<svc>.yaml` / 120（:67-70） | 各服务的 `${spring.application.name}.yaml` | 不发 | server.port、Kafka 消费组、预约调度、Feign 等业务配置都在这里 |
| go-service-dev.yaml | 同名 / 120 | Go config.go:187,199,306；命名空间来自 `NACOS_NAMESPACE_ID`，默认 120（:178） | 不发 | DSN 含明文密码（:24-34） |
| middleware-cluster.yaml、postgresql-cluster.yaml、redis-cluster.yaml、database-connection.yaml、cache-connection.yaml、messaging-connection.yaml、cluster-switch.yaml | 同名 / 120 | 在 couponkill-\*/src/main grep 不到任何读取。order 和 user 监听的是 `middleware-cluster-config.yaml`（OrderShardingSphereConfig.java:117、UserShardingSphereConfig.java:118），名字对不上 | 往 public 发内联版（:182,256,332,472,528,581,630） | 推断是没有消费方的"说明性"配置 |
| service-collaboration（没有扩展名） | 不导入（`-Filter *.yaml`，:63） | 未发现消费方 | 不发 | 顶层键叫 `service协同`；`go.enabled` 和 `fallback-to-go` 都是 true，与"Go 热路径冻结"的说法相反 |
| KAFKA_GROUP/kafka-config.yaml | 不导入 | 未发现消费方 | 不发 | 消费组名和代码一致（:20-22） |
| SENTINEL_GROUP/couponkill-gateway-sentinel | couponkill-gateway-sentinel / SENTINEL_GROUP / 120（:81） | gateway 的 Sentinel 数据源（couponkill-gateway-dev.yaml:26-32、common.yaml:41-49） | 不发 | 规则只有 coupon-service、order-service、seckill-go-service；网关路由上的 user-service、connector-service 没有规则 |
| shard/DEFAULT_GROUP/{user,order,coupon}-service-sharding.yaml | 同名 / DEFAULT_GROUP / 998（:73） | {User,Order,Coupon}ShardingSphereConfig，读 `shardingsphere.namespace=998`（如 user application.yml:24-25） | 不发 | 2 库 x 16 表；`maximumPoolSize: 32`，和 order-dev 注释里写的"=300"不一致 |
| values-prod.yaml 的 `nacos.config.*.content`（:89-440） | - | 只被 deploy-\*.yaml 当作 sha256 注解的输入 | nacos-init 不引用 | 第三份分叉副本：分片是 4 库，路由写成 `couponkill-service:808x` 加 `StripPrefix=3` |

命名空间约定：代码的 config 和 discovery 期待 120（Helm 里的 gateway 例外，被设成 public），ShardingSphere 期待 998。本地导入脚本会创建这两个命名空间，Helm 一个都不建。

### 4.4 Kafka

| 项 | 现状 | 证据 |
|---|---|---|
| bootstrap | Java 用 `${KAFKA_BOOTSTRAP_SERVERS:kafka:9092}`；Helm 提供 Service `kafka:9092` 和 `broker:9092`；advertised 为 `PLAINTEXT://kafka.<ns>.svc.cluster.local:9092`；compose 容器内 `kafka:29092`，宿主 `localhost:9092` | nacos/DEFAULT_GROUP/common.yaml:7；templates/kafka-statefulset.yaml:29-59,99；docker-compose.migration.yml:40-58 |
| topic | Chart：seckill_order_create、seckill_order_result、seckill_compensation，16 分区，RF 1。compose 还多建一个 order_created，而 order 服务确实在用这个 topic | values.yaml:225-229；docker-compose.migration.yml:75；couponkill-order-service/.../service/Impl/OrderServiceImpl.java:171 |
| 代码里的消费组 | seckill-order-create-group、seckill-order-result-group | couponkill-order-service/.../listener/SeckillOrderCreateListener.java:24、SeckillOrderResultListener.java:25；nacos/KAFKA_GROUP/kafka-config.yaml:20-22 |
| KEDA 里的配置 | values.yaml 用 seckill-go-group / order-create；canary 用 go-dispatcher / order-create，topic 写成 `seckill.order.create` | values.yaml:404-413；values.canary-keda.yaml:173-191 |
| 安全 | PLAINTEXT，没有 SASL，也没有 TLS | kafka-statefulset.yaml:95-104 |

### 4.5 数据库（由脚本创建）

| 数据库 | 表 | 脚本 | Helm 是否执行 | compose 是否执行 |
|---|---|---|---|---|
| user_db_0、user_db_1 | "user"、user_coupon_count | init-postgres.sql:16-17,28-80 | 只有 db.init.enabled 时才执行（默认 false，prod true） | 执行（01） |
| order_db_0、order_db_1 | order_0 到 order_15（索引 user_id、coupon_id、virtual_id、create_time）；order_db_0 另有 seckill_reservation、user_notification | init-postgres.sql:84-195 | 同上 | 执行（01） |
| coupon_db_0、coupon_db_1 | coupon_0 到 coupon_15（主键 id + shard_index，带 seckill_start_at/end_at）、stock_log_0 到 stock_log_15 | init-postgres.sql:199-300 | 同上 | 执行（01） |
| connector_db | platform_sku_binding、coupon_price_map | 03-init-connector.sql:4-41 | 不执行 | 执行（03） |
| 演示数据 | user_db_0 的用户、coupon_db_0 的券 | 02-seed-demo.sql | 不执行 | 执行（02） |
| 存量库迁移 | 补列、补表 | 04-seckill-reservation.sql | 不执行 | 不执行 |
| MySQL 旧库 | user、coupon、order、user_coupon | init.sql.deprecated-mysql | 不执行 | 不执行 |

### 4.6 Secret 与集群前置条件

| 前置条件 | 使用方 | 是否 optional | Chart 是否创建 | 是否有文档 |
|---|---|---|---|---|
| Secret `jwt-secret`，key `secret` | gateway（deploy-gateway.yaml:77-81）、user（deploy-user.yaml:92-96） | 否，缺失会导致 CreateContainerConfigError | 否 | 仓库 \*.md 里 grep 不到 |
| Secret `connector-secrets`，key `internal-token` | connector（deploy-connector.yaml:60-65） | 渲染结果永远是 `optional: true` | 否 | 无 |
| 外部 PostgreSQL，挂在 DNS `postgres` 上（Nacos 配置还引用 postgres-master、postgres-slave-0/1） | 全部 Java/Go 服务、db-init | - | 只建了没有 selector 的 Service | 没说要手工建 Endpoints |
| 外部 Redis，挂在 `redis` / `redis-master` 上 | 全部服务 | - | 同上 | 无 |
| 镜像拉取凭据（阿里云个人版 ACR） | 全部业务镜像 | - | 模板里没有 imagePullSecrets | 无（仓库是否公开未验证） |
| TLS Secret `couponkill-credential`（istio-system） | k8s-istio/istio_gateway.yaml:43 | - | 否 | 无 |
| CRD 前置：Istio、KEDA、Prometheus Operator | istio.enabled、keda.enabled、monitoring | - | 否 | README 只列了版本号 |

### 4.7 HTTP 暴露面

| 入口 | 路径 | 目标 | 鉴权 |
|---|---|---|---|
| Chart 的 Istio Gateway（HTTP 80，host `istio.gateway.host`） | /seckill、/api/v1/user、/api/v1/order、/api/v1/coupon、/nacos、/sentinel（templates/istio.yaml:47-138） | 直连各服务 | 不经过 Spring Gateway，没有 JWT 校验 |
| Service `nacos-external`（prod，LoadBalancer 8848，values-prod.yaml:73-78） | Nacos 全部 OpenAPI 和控制台 | nacos | Helm 没配 Nacos 鉴权 |
| k8s-istio Gateway（istio-system，hosts `*`，80 与 443） | 各服务的 VS，外加 nacos-vs、sentinel-vs（istio_VirtualService.yaml:199-264） | 直连 | AuthorizationPolicy 只校验来源 SA |
| k8s-nothing NodePort + Ingress | /api/v1/\*、/seckill、/gateway、/user、/order、/coupon、/go | 一体 Pod | - |
| 探针 | Java `/actuator/health`；Go `/health`；operator `/healthz`、`/readyz`；Kafka exec kafka-topics.sh；Nacos 没有探针 | | |
| 初始化脚本调用的 Nacos API | `/nacos/v1/console/health/liveness`（nacos-init-job.yaml:28）、`/nacos/v1/cs/configs`（nacos-init 与本地脚本）、`/nacos/v1/console/namespaces`（import-nacos-local.ps1:20） | | 无 |

### 4.8 镜像命名：生产者与消费者

| 制品 | Makefile | build.ps1 | Jenkinsfile | Chart 期望 |
|---|---|---|---|---|
| 业务镜像 | `${REGISTRY}:<svc>`（Makefile:164），6 个，不含 connector | `${REGISTRY}:<svc>`（build.ps1:70,79），7 个 | `${REGISTRY}/<svc>:${BUILD_NUMBER}`（Jenkinsfile:38,85），7 个 | `<image.registry>:<services.<svc>.image.name>`（如 deploy-gateway.yaml:31） |
| 金丝雀镜像 | `${CANARY_REGISTRY}:<svc>`（Makefile:193） | 同左（build.ps1:102） | `${CANARY_REGISTRY}/<svc>:canary`（Jenkinsfile:39） | canary values 用 registry `.../canary-keda-dev`，拉 `canary-keda-dev:<svc>` |
| 依赖镜像 | `${REGISTRY}:postgres` 等（Makefile:216） | `${REGISTRY}/postgres` 等（build.ps1:139-140） | `${REGISTRY}/postgres` 等（Jenkinsfile:112） | prod 的 init 镜像要 `my-docker:redis`、`my-docker:kafka`；Sentinel 渲染成 `my-docker:sentinel-dashboard:1.8.5`（非法引用） |

结论：只有 Makefile 和 build.ps1 的"标签式"命名与 Chart 对得上，而它们推的是可变 tag。CD 真源 Jenkins 用的是"路径式 + 构建号"，没有任何模板会引用。Makefile 注释里写着"阿里云个人仓库使用标签区分镜像"，所以路径式推送在个人版 ACR 上本身也可能被拒绝（推断）。

### 4.9 values 接口：活键与死键

- 活键（模板确实会引用）：`namespace`、`namespaceMonitor`、`crd.install`、`examples.seckill`、`dependencies.enabled`、`db.*`（host、port、username、password、database、init.\*）、`image.registry`、`services.<svc>.{name,port,replicas,image.name,jvmOpts,threadPoolOpts,resources,hpa.{enabled,minReplicas,maxReplicas,cpu},goOpts}`、`services.connector.{enabled,profile,internalToken*}`、`redis.{enabled,host,port,password,cluster.enabled,cluster.type,init.*}`、`kafka.{enabled,name,port,broker,bootstrapServers,topics,partitions,replicationFactor,image,statefulSet.*,cluster.enabled,init.*}`、`postgres.cluster.{enabled,type}`、`nacos.{enabled,name,image,replicas,cluster.*,service.*,config.external.*,config.<x>.content（只当 checksum）,configWatcher.*,storage.type}`、`sentinel.*`、`serviceAccount0-4`、`keda.enabled`、`keda.kafka.*`、`monitoring.enabled`、`monitoring.prometheus.enabled`、`canary.<svc>.weight*`、`istio.{enabled,gateway.{name,host,port},destinationRule.*}`、`config.refreshInterval`、`dynamic.*`。
- 死键（三份 values 里存在，但 grep 模板引用为 0）：`image.tag`、`services.<svc>.hpa.targetCPUUtilizationPercentage`、`redis.master.*`、`postgres.{enabled,name,port,rootPassword,database,image,resources,cluster.masterSlave}`、`db.cluster.*`、`nacos.config.init.*`、`nacos.storage.postgresql.*`（模板只认 `mysql`）、`istio.virtualService.*`、`istio.gateway.tls`、`canary.<svc>.{enabled,stableSubsetLabel,canarySubsetLabel}`、`keda.scaledObject`、`keda.triggers`、`monitoring.serviceMonitor`、`monitoring.prometheusRule`、`networkPolicy.*`、`security.*`、`logging.*`、values-prod 里 `nacos.config.{collaboration,middleware,gateway,go}` 的 dataId/group。

## 5. 依赖关系

### 5.1 部署路径盘点

| 路径 | 文档定位 | 覆盖的业务服务 | 覆盖的中间件 | 完整度（现状） | 维护状态（git） |
|---|---|---|---|---|---|
| Helm `charts/couponkill` | 生产/演示唯一真源 | gateway、user、coupon、order、connector、seckill-go、operator（含 CRD） | Kafka（STS）、Nacos（Deployment 或 STS）、Sentinel（prod）；PG 和 Redis 只有占位 Service | lint/template 能过；运行期多处断裂（R02 至 R20） | 活跃：templates 20 次提交、values.yaml 22 次，最近一次 2026-07-18 |
| `docker-compose.migration.yml` + `scripts/import-nacos-local.ps1` | 本地联调 | 不含应用，应用跑在宿主机上 | PG 16、Redis 7、Kafka 3.8.0、Nacos 3.1.1 | 自洽；config 校验通过 | 2 次提交，最近一次 2026-07-18（改为 5433 映射） |
| `k8s-istio/` | Helm 之上的叠加样例 | 通过 VS/DR/AuthZ 覆盖 go、order、coupon、user、gateway、nacos、sentinel | - | 与 Chart 在对象名、Gateway 位置、SA 上多处冲突（R29、R36） | 4 次提交，最近一次 2026-07-18 |
| `k8s-nothing/` | DEPRECATED | 5 个服务塞进 1 个 Pod | MySQL、Redis、RocketMQ、Nacos（MySQL 存储）、Sentinel | 历史形态，和现状不符 | 3 次提交，最近一次只改了废弃横幅 |
| `cross-namespace-monitoring/` | DEPRECATED | 5 个 ServiceMonitor | - | 抓取的端点并不存在（R30） | 2 次提交，最近一次只改了横幅 |
| `couponkill-operator/`（kustomize `config/`，以及 Chart 里的 deploy-operator.yaml） | 可选 | 通过 Seckill CR 编排 | - | 两条安装路径、两份 CRD（R26） | 6 次提交，最近一次 2026-07-18 |
| `ansible/` | DEPRECATED | 在 ECS 上用 docker-compose 部署（deploy_service 角色的模板） | 未深读 | 未深读 | 4 次提交，最近一次只改文档 |
| `Jenkinsfile` | CD 真源 | 7 个镜像、2 个 helm release | 同步 5 个中间件镜像 | 构建、推送、部署三处断裂（R01、R04、R05、R07） | 13 次提交，最近一次 2026-07-18 |

`docs/DEPLOYMENT-SOURCE-OF-TRUTH.md` 和 `docs/CICD-SOURCE-OF-TRUTH.md` 把"谁是真源"说清楚了，各 DEPRECATED 目录的 README 也都加了横幅（k8s-nothing/couponkill-simple/README.md:3-4、cross-namespace-monitoring/README.md:3-4、k8s-istio/README.md:3-4）。但文档对真源能力的描述和实际文件不一致：

- 文档说 Chart 负责"服务、中间件、Nacos init"。实际上 Chart 不部署 PG/Redis，nacos-init 也发不出应用需要的配置（R10、R14）。
- 变更纪律第 1 条说"只改 charts/couponkill（及必要的 Nacos 仓库副本 nacos/）"。但 Helm 只能读 chart 目录内的文件，Chart 根本读不到仓库根的 `nacos/`，这两份副本之间没有任何同步机制（R11）。
- CICD 文档说"镜像 tag/registry 变量在 Jenkins 与 Makefile/build.ps1 之间对齐"。实际上两边一个是路径式，一个是标签式（4.8）。
- CICD 文档说 ci.yml 做"父工程 -am 编译 + 单元/契约测试"。实际上 `-pl` 列表里没有 connector-service（ci.yml:27）。

### 5.2 各部署路径对中间件的对接名

| 依赖 | compose | Helm Service | Nacos 配置里的引用 | 本地导入脚本改写成 | k8s-nothing |
|---|---|---|---|---|---|
| PostgreSQL | postgres:5432（宿主 5433） | postgres、postgres-master、postgres-slave（没有 selector 的 headless） | 分片配置用 `postgres:5432`；common、middleware-cluster、postgresql-cluster 用 postgres-master、postgres-slave-0/1、postgres-node-0 至 2 | 127.0.0.1:5433 | storage-service:3306（MySQL） |
| Redis | redis:6379 | redis、redis-master、redis-slave（没有 selector） | common.yaml:21 `redis-master`；go-service-dev.yaml:37 `redis-master:6379`；redis-cluster 与 middleware 里还有 redis-slave-0/1、redis-sentinel-0 至 2 | 127.0.0.1 | storage-service:6379 |
| Kafka | kafka:29092 / localhost:9092 | kafka:9092、broker:9092（STS） | common.yaml:7 `kafka:9092`；messaging-connection、kafka-config 也是 `kafka:9092`；values-prod 的正文用 `broker:9092` | 127.0.0.1:9092 | RocketMQ（software-service） |
| Nacos | nacos:8848、9848 | nacos:8848（两份，都没有 9848） | nacos:8848；各 \*-dev.yaml 里写的 namespace 是 `couponkill` | 127.0.0.1:8848 | software-service:8848 |
| Sentinel | 无 | sentinel-dashboard:8080（prod，两份） | sentinel-dashboard:8080 | 不改写 | software-service |

### 5.3 与其他分片的接口点

- java-foundation / java-business：application.yml 里 Nacos 地址和命名空间的占位写法（R13）；UserContextFilter 信任请求头（R16）；actuator 暴露范围和 Prometheus 依赖（R30）；`middleware-cluster-config.yaml` 这个监听 dataId 在任何地方都不存在（4.3）；gateway 是否会剥离客户端传入的 `X-User-*`（本分片没有核实）。
- go-operator：Go 实际读哪些环境变量、默认端口 8090（R31）；operator 的 flag 与探针地址（R25）；Chart 与 operator 的 CRD schema 分叉（R26）。
- ops-tooling：`scripts/import-nacos-local.ps1`、`scripts/local-http-smoke.ps1` 是本地链路唯一的"真实验证"；ansible 的 ECS 路径。
- crosscut-e2e：端口矩阵（4.1）、Nacos 命名空间约定（4.3）、Kafka topic 与消费组（4.4）。

## 6. 问题与风险

编号供其他分片和汇总步骤引用。「推断」表示没有在集群或流水线上执行过。

| # | 严重度 | 类别 | 证据 path:line | 说明 | 建议方向 |
|---|---|---|---|---|---|
| R01 | 高 | bug | couponkill-gateway/Dockerfile:2；couponkill-{user,coupon,order,connector}-service/Dockerfile:2 | builder 镜像 `maven:3.9.9-eclipse-temurin-25` 在 Docker Hub 不存在（已核实：3.9.9 系列最高到 temurin-24）。除非本地或私有源恰好有同名镜像，否则所有 Java 镜像在拉基础镜像这一步就会失败 | 改成 `maven:3.9.11-eclipse-temurin-25` 或更新版本，并 pin digest；CI 加 docker build |
| R02 | 高 | bug | charts/couponkill/templates/deploy-coupon.yaml:39 | `{{ .Values.services.coupon.image.name }}}` 多了一个 `}`，三份渲染结果里都是 `.../my-docker:coupon}`（已核实）。这是非法镜像引用，会报 InvalidImageName，秒杀热路径的 coupon 起不来 | 删掉多余的 `}`；CI 加渲染校验（kubeconform） |
| R03 | 高 | 一致性 | deploy-coupon.yaml:1-91（只有 Deployment）；templates/istio.yaml:96-108；virtualservice-canary.yaml:15-24；monitoring.yaml:56-69；values-prod.yaml:514-526 | coupon 没有 Service，也没有 HPA 模板。Istio 路由、canary 子集、ServiceMonitor、prod 的 HPA（3 到 10 副本）指向的都是不存在的对象 | 照 gateway 模板补齐 Service 和 HPA |
| R04 | 高 | 一致性 | Jenkinsfile:38,85,141；deploy-gateway.yaml:31（其他 deploy-\*.yaml 同理）；values.yaml:56；Makefile:164；build.ps1:70 | Jenkins 推 `.../my-docker/<svc>:${BUILD_NUMBER}`（路径式），Chart 拉 `.../my-docker:<svc>`（标签式）。模板不读 `image.tag`（grep 只命中注释），所以 `--set image.tag=${BUILD_NUMBER}` 不起作用。集群里跑的是最后一次由 Makefile/build.ps1 推上去的可变 tag，没法按构建号回滚 | 统一命名：个人版 ACR 用标签式时 tag 设为 `<svc>-<build>`；模板引用 tag |
| R05 | 高 | bug | Jenkinsfile:28；couponkill-operator/ 下没有 Makefile（已核实）；Makefile:52,74 | Jenkins 的 Build Operator 阶段执行 `cd couponkill-operator && make generate`，找不到 Makefile，Build 阶段失败，后面的镜像和部署都不会执行。根目录 Makefile 的 generate/manifests/build 针对的是仓库根的 `./...` 和 `main.go`，而根目录既没有 go.mod 也没有 main.go | 在 operator 目录放一份独立 Makefile，或者 Jenkins 直接调用 controller-gen / go build |
| R06 | 高 | bug | templates/couponConfig.yaml:1-12；Jenkinsfile:141,149；Makefile:135,139,143；build.ps1:44,49,54 | Chart 自带 Namespace `couponkill`（和 `monitor`），而所有部署命令都带 `--create-namespace`。Helm 先建出一个没有归属标签的 namespace，再创建 Chart 里的同名 Namespace 时会报 ownership 冲突（推断，高置信）。另外，卸载 release 会把 `monitor` 命名空间一起删掉 | 从 Chart 里去掉 Namespace；或者去掉 `--create-namespace`，并给 Namespace 加开关 |
| R07 | 高 | bug | Jenkinsfile:149 | canary 被当成第二个 release `couponkill-canary`，部署到同一个命名空间。它渲染出的 Namespace、CRD、ClusterRole、Deployment、Service 与主 release 同名，Helm 所有权冲突，这个阶段必然失败（推断，高置信）。就算成功，也只是把整套对象覆盖一遍，并不存在真正的 canary 工作负载 | 在同一个 release 里做 canary Deployment（带 `version=canary` 标签）加权重分流 |
| R08 | 高 | bug | dependencies.yaml:93-104 与 nacos.yaml:141-157；dependencies.yaml:107-118 与 sentinel.yaml:3-19；istio.yaml:32-35 与 virtualservice-canary.yaml:7 | 同一个 release 里有同名对象：`Service nacos`（三份渲染都有）、`Service sentinel-dashboard`（prod）、`VirtualService couponkill-vs`（istio.enabled 时）。Helm 3 是逐个 Create 的，第二个会报 AlreadyExists（推断）。就算能互相覆盖，一个是没有 selector 的 headless，一个是 ClusterIP，spec 本身互斥 | 删掉 dependencies.yaml 里 nacos 和 sentinel 的占位；两个 VS 合并成一个 |
| R09 | 高 | bug | kafka-init-job.yaml:32-47；对照 kafka-statefulset.yaml:122、docker-compose.migration.yml:67-80 | kafka-init 在 apache/kafka 镜像里调用 `kafka-topics`（这是 Confluent 镜像的命名，apache 镜像里只有 `/opt/kafka/bin/kafka-topics.sh`），还在 `/bin/sh` 下用了 `read -ra` 和 `<<<`。`until` 循环永远不会结束。它是 post-install hook 且默认启用（values.yaml:251），会一直卡到超时，release 变成 FAILED；按名字排在它后面的 nacos-init 永远不会执行（推断，高置信） | 与 compose 对齐：用 `/bin/bash` 和绝对路径；hook 加 weight 和 backoffLimit |
| R10 | 高 | 一致性 | values.yaml:315-318；nacos-init-job.yaml:35-67,77,152；各服务 application.yml 的 namespace 行（如 user:17,22,25）；import-nacos-local.ps1:7-8,63-75 | Helm 提供不了应用需要的 Nacos 配置。默认 `config.external.enabled=true`，会去下载一个占位 URL；fallback 分支只往 public 命名空间写一份内联副本，里面没有 `couponkill-<svc>.yaml`、没有 `gateway-routes.yaml`（误写成了 `gateway.yaml`）、也没有 `*-service-sharding.yaml`，而应用读的是命名空间 120 和 998。结果是 K8s 下 ShardingSphere 数据源、网关路由、server.port、Kafka 消费组配置全部缺失 | 把 `nacos/` 复制进 chart，用 `.Files.Glob` 生成导入 Job，按 120/998 发布；或者统一命名空间约定 |
| R11 | 高 | 一致性 | nacos/（仓库副本）；nacos-init-job.yaml:69-700（内联副本）；values-prod.yaml:89-440（第三份，只用来算 checksum） | Nacos 配置有三套互相分叉的"真源"。分片数（2 库 vs values-prod 的 4 库）、路由目标（`lb://<服务名>` vs `couponkill-service:808x` 加 `StripPrefix=3`）、dataId（gateway-routes.yaml vs gateway.yaml，middleware-cluster.yaml vs middleware-cluster-config.yaml）都对不上。DEPLOYMENT-SOURCE-OF-TRUTH 要求"改 charts 及必要的 nacos/ 副本"，但 Chart 根本不读 `nacos/` | 只保留一份（建议 `nacos/`），由 Chart 通过文件引用生成 |
| R12 | 高 | bug | nacos.yaml:150,208；dependencies.yaml:97-103；对照 docker-compose.migration.yml:98-99、pom.xml:70 | Helm 里 Nacos 的 Service 和容器只暴露 8848，没有 gRPC 端口 9848。nacos-client 3.1.1 按"主端口 + 1000"连 gRPC，所以 K8s 内的客户端连不上 Nacos（推断，高置信）。集群模式下连 `nacos` 这个客户端 Service 都没有（已核实渲染结果） | Service 和容器补上 9848（集群模式再加 9849）；集群模式补一个 ClusterIP Service |
| R13 | 高 | 一致性 | deploy-gateway.yaml:62-65；deploy-user.yaml:83-87；deploy-order.yaml:48-51；deploy-connector.yaml:48-51；各服务 application.yml:13-23 | Nacos 地址和命名空间的注入方式不统一。只有 gateway 注入了应用 yml 真正读取的 `NACOS_SERVER_ADDR`，而且把命名空间设成了 public。user、order、connector 只注入 `SPRING_CLOUD_NACOS_SERVER_ADDR`，但 yml 里显式写了 `config.server-addr` 和 `discovery.server-addr: ${NACOS_SERVER_ADDR:localhost:8848}`，推断它们会去连 localhost。除 gateway 外命名空间都默认 120，于是网关在 public 里找服务，服务却注册在 120，`lb://` 路由找不到实例（推断，高置信） | 所有 Java 服务统一注入 `NACOS_SERVER_ADDR` 和 `NACOS_NAMESPACE` |
| R14 | 高 | 一致性 | dependencies.yaml:1-73；values.yaml:193-212,262-290；charts/couponkill/README.md「依赖服务管理」；docs/DEPLOYMENT-SOURCE-OF-TRUTH.md「唯一生产入口」 | Chart 不部署 PostgreSQL 和 Redis，只有没有 selector、也没有 Endpoints 的 headless Service；`redis.master.*`、`postgres.enabled` 这些键没有任何模板引用。文档却说 Chart 负责"服务、中间件"。Nacos 配置里还指向 `redis-master`、`postgres-master`、`postgres-slave-0/1` 这些根本不存在的名字 | 文档写清楚"PG/Redis 外置，需要手工建 Endpoints 或 ExternalName"；或者引入成熟的子 chart |
| R15 | 高 | bug | templates/network.yaml:3-39；values-prod.yaml:680-715（没有模板引用） | NetworkPolicy 无条件渲染。default-deny 同时拒绝 Ingress 和 Egress，但没有任何 egress 放行，DNS 也被挡住；allow-istio-system 依赖命名空间标签 `name: istio-system`，而默认只有 `kubernetes.io/metadata.name` 这个标签。在会执行 NetworkPolicy 的 CNI 上，所有 Pod 和 hook 都会断网。kind、minikube 默认的 CNI 不执行 NetworkPolicy，所以本地看不出问题 | 加 egress 放行（kube-dns、同命名空间、istio-system），改用 metadata.name 标签，并加开关 |
| R16 | 高 | 安全 | templates/istio.yaml:47-138；couponkill-common/.../filter/UserContextFilter.java:28-33；couponkill-connector-service/.../config/ConnectorAdminInterceptor.java:56-59；couponkill-gateway/.../security/JwtAuthGlobalFilter.java:114-118 | Istio 入口把 /api/v1/user、order、coupon 直接路由到各服务，不经过负责 JWT 校验、注入身份头的 Spring Gateway。下游把调用方传来的 `X-User-Id` 当作已认证用户，把 `X-User-Roles: admin` 当作管理员。只要 istio.enabled（prod 默认开），就能伪造任意用户和管理员（推断，高置信） | 入口只路由到 couponkill-gateway；或者用 RequestAuthentication 加 AuthorizationPolicy，并在边缘剥掉 `X-User-*` |
| R17 | 高 | 安全 | templates/istio.yaml:113-137；values-prod.yaml:73-78；docker-compose.migration.yml:92；nacos/DEFAULT_GROUP/couponkill-gateway-dev.yaml:91 等 | Nacos 里存着明文 DB 密码和 JWT secret，却经 Istio 的 `/nacos` 路由对外开放，prod 还额外建了 `nacos-external` LoadBalancer。Helm 没配任何 Nacos 鉴权变量，等于公网可读配置；拿到 `jwt.secret` 就能签发任意令牌 | 删掉 `/nacos`、`/sentinel` 路由和 LoadBalancer；开启 Nacos 鉴权；把秘密移出 Nacos |
| R18 | 高 | 安全 | k8s-nothing/couponkill-simple/couponKill.yaml:9-15（key `.dockerconfigjson`，base64 明文，长度 296） | 仓库里提交了一个镜像仓库拉取凭据 Secret。目录虽然标了 DEPRECATED，但 git 历史会永久保留 | 立刻轮换 ACR 凭据；清理 git 历史；改用集群侧的 imagePullSecret |
| R19 | 高 | bug | values-prod.yaml:514；deploy-coupon.yaml 的 env 段（没有 SERVER_PORT）；nacos/DEFAULT_GROUP/couponkill-coupon-service-dev.yaml:2 | prod 把 coupon 端口设为 8081（用于 containerPort、探针和 Service 目标端口）。但 coupon 没有注入 SERVER_PORT，实际监听的是 Nacos 里的 8080 或 Spring 默认的 8080，探针会失败，进入 CrashLoop（推断） | 所有 Java 服务统一注入 SERVER_PORT；收敛端口矩阵（4.1） |
| R20 | 高 | bug | values-prod.yaml:72；nacos.yaml:187,210-211,239-241 | prod 设了 `nacos.replicas: 3`，但仍然是 `MODE=standalone` 加 emptyDir。结果是 3 个互不相干的单机 Nacos 挂在同一个 Service 后面轮询，nacos-init 写进哪一个是随机的。任何一次 Nacos 重启都会丢掉全部配置，而 nacos-init 只在 install/upgrade 时运行 | 单机模式就用 1 个副本加 PVC；要多副本就走 cluster 模式加外部数据库 |
| R21 | 中 | 一致性 | docker-compose.migration.yml:91-95；nacos.yaml:209-231 | compose 的注释写明 Nacos 3.x 镜像强制要求 `NACOS_AUTH_TOKEN` 和 `NACOS_AUTH_IDENTITY_*`，Helm 的 Nacos 容器一个都没配，可能根本起不来（推断，依据是仓库自己的注释） | 在 values 里提供，值从 Secret 引用 |
| R22 | 中 | 安全 | deploy-gateway.yaml:77-81；deploy-user.yaml:92-96；deploy-connector.yaml:60-65 | 依赖 Secret `jwt-secret`（非 optional）和 `connector-secrets`，但 Chart 不创建，文档也没提（grep \*.md 无命中）。gateway 和 user 会停在 CreateContainerConfigError | 写进文档；或者由 Chart 按条件创建，用 lookup 保留已有值 |
| R23 | 中 | bug | deploy-connector.yaml:1,65,67；deploy-user.yaml 的 CONFIG_REFRESH_ENABLED 行；values-prod.yaml:571 | `x \| default true` 会把 false 当成空值。connector 关不掉（已核实渲染）；prod 的 `optional: false` 被渲染成 true（已核实）；`internalTokenStrict: false` 和 `dynamic.config.enabled: false` 同样不生效 | 改用 `hasKey` 或 `ternary` 判断布尔值 |
| R24 | 中 | bug | deploy-gateway.yaml:110-129（其他 deploy-\*.yaml 同理）；values.yaml:71,92,113,134,178,190 | 模板读的是 `hpa.cpu`，values.yaml 给的却是 `targetCPUUtilizationPercentage`。用默认 values 开 HPA 时渲染出空的 `averageUtilization`（已核实），HPA 会被 API Server 拒绝 | 统一键名，并给默认值 |
| R25 | 中 | bug | deploy-operator.yaml:33；couponkill-operator/main.go:41-42 | `--health-probe-bind-address=:{{ port \| quote }}` 渲染成 `:"8084"`（已核实），监听地址非法，manager 启动会失败（推断）。metrics 还在默认的 :8080，`METRICS_PORT` 没人读。Operator 模板没有开关，总会被部署 | 去掉 quote；加 `services.operator.enabled` |
| R26 | 中 | 一致性 | templates/crd.yaml:36-66；templates/seckill-example.yaml:8-31；couponkill-operator/config/crd/bases/ops.couponkill.io_seckills.yaml:42,57,105 | Chart 里的 CRD（必填 couponId、startTime、endTime、totalStock）和 operator 生成的 CRD（services、scaling、monitoring）不是同一个 schema。Chart 自带的示例是按 operator 的 schema 写的，缺必填字段，会被 Chart 的 CRD 拒绝。CRD 放在 templates/ 下，卸载时会连同所有 CR 一起删掉 | 从 operator 的 `config/crd/bases` 生成 CRD，放进 `crds/` 目录 |
| R27 | 中 | 一致性 | values.canary-keda.yaml:167-191；values.yaml:401-413；SeckillOrderCreateListener.java:24；values-prod.yaml:655-668 | KEDA 配置和代码对不上。canary 用的 topic 是 `seckill.order.create`（实际是 `seckill_order_create`），消费组是 `go-dispatcher` 和 `order-create`（实际是 `seckill-order-create-group`），目标 Deployment 是 `couponkill-go-service`（实际是 `seckill-go`）。prod 的 `keda.scaledObject` 和 `keda.triggers` 没有模板引用，最后只渲染出一个空的 TriggerAuthentication | 以代码为准修改 values；删掉死键 |
| R28 | 中 | 一致性 | destinationrules.yaml:12-38；deploy-gateway.yaml:12,22；deploy-order.yaml:7-17；virtualservice-canary.yaml:34-50 | 金丝雀机制根本不成立。DR 子集要求 `version=stable` 或 `version=canary`，但 Pod 上的标签是 `version: v1` 或者干脆没有；Chart 不渲染任何 canary 工作负载；canary VS 用 `/api/v1` 兜底，会把 user 和 coupon 的请求送到 order；`seckill` 前缀少了 `/`，永远匹配不上 | 见 R07 |
| R29 | 中 | 一致性 | deploy-user.yaml:42；deploy-order.yaml:20；deploy-serviceAccount.yaml；deploy-connector.yaml（没有 SA）；k8s-istio/istio_AuthorizationPolicy.yaml:30-176 | user 的 Pod 用的是 order 的 SA（serviceAccount2），user 自己的 SA 闲置；connector 用的是 default SA。基于 SA 的 Istio 身份和 AuthorizationPolicy 因此全部错位。k8s-istio 的 ALLOW 列表里也没有 Spring gateway 的 SA（couponkill-gateway-service）和 connector | user 改用 serviceAccount3；connector 加 SA；按实际 SA 重写策略 |
| R30 | 中 | 一致性 | 所有 couponkill-\*/pom.xml（grep micrometer 为 0）；各服务 application.yml 的 management 段（如 order:37-42）；monitoring.yaml:5-87；deploy-order.yaml:80 | 指标抓取用不了。Pod 注解和 ServiceMonitor 都去抓 `/actuator/prometheus`，但既没有 Prometheus registry 依赖，端点也没有暴露；order 和 connector 的 Service 端口没有名字（ServiceMonitor 要求 `port: http`）；go 的 ServiceMonitor 放在 `monitor` 命名空间，又没有 namespaceSelector；prod 的 `monitoring.prometheus.enabled` 继承了 false，什么都不渲染 | 加依赖和 exposure；给端口命名；统一 monitor/monitoring 命名空间 |
| R31 | 中 | 一致性 | deploy-go.yaml:66-100；couponkill-go-service/internal/config/config.go:173-178,187,362；couponkill-go-service/Dockerfile:44,51 | Go 的 Helm 环境变量基本都是死的。代码只读 `NACOS_SERVER_ADDR`、`NACOS_ADDR`、`NACOS_NAMESPACE_ID`（以及 `GO_DB_MAX_*`）；`PORT`、`REDIS_ADDR`、`POSTGRES_DSN`、`KAFKA`、`NACOS_CONFIG_*` 和各个 `*_CLUSTER_*` 在代码里 grep 都是 0。K8s 里命名空间 120 不存在，服务会退回默认端口 8090，而探针打的是 8083。Go 模板没有开关，总会被部署，和 README 说的"默认关闭"不符 | 以代码为准精简 env；加 enabled 开关 |
| R32 | 中 | bug | db-init-job.yaml:42,66-67；docker-compose.migration.yml:22-24；values.yaml:137 | Helm 的 db-init 只执行 init-postgres.sql，不会建 connector_db（那是 03 脚本的事）。connector 默认是启用的，却没有库可连。04 迁移脚本没有任何入口会执行 | db-init 挂载并按顺序执行 01、03（02 可选） |
| R33 | 中 | bug | kafka-statefulset.yaml:84-146 | PVC 挂在 `/var/lib/kafka/data`，但没有设 `KAFKA_LOG_DIRS`，而 apache/kafka 默认的日志目录不在这个路径下。于是数据和 KRaft 元数据都不落在 PVC 上，Pod 重建就会丢 topic（推断，中置信）。另外 chart 的 topic 列表里没有 `order_created`，只能靠自动建 topic，分区数和 compose 不一样 | 设置 `KAFKA_LOG_DIRS`；topic 列表补上 order_created |
| R34 | 中 | 安全 | values.yaml:25,265,363；values-prod.yaml:23,87 以及 nacos.config 正文；deploy-user.yaml:77；deploy-order.yaml:33；deploy-go.yaml:73；nacos-init-job.yaml 的内联配置；nacos/DEFAULT_GROUP/common.yaml:83,88,92；nacos/shard/DEFAULT_GROUP/\*.yaml:7,19；go-service-dev.yaml:24-34；middleware-cluster.yaml、postgresql-cluster.yaml 的 password 键；couponkill-gateway-dev.yaml:91、couponkill-coupon-service-dev.yaml:30、couponkill-user-service-dev.yaml:34 的 `jwt.secret`；connector application.yml:58 的 internal-token 默认值；docker-compose.migration.yml:17,94（仅本地） | 明文凭据散落在 values、模板和 Nacos 副本里（这里只列了 key 名，没有复制值） | 统一走 K8s Secret 或外部密钥管理；Nacos 里只放引用 |
| R35 | 中 | 安全 | 5 个 Java Dockerfile（都没有 USER）；Chart 业务 Deployment 都没有 securityContext（grep 只命中 operator 和 nacos-init）；values.yaml:56,201,344；couponkill-go-service/Dockerfile:23；deploy-gateway.yaml:32 等 | Java 容器以 root 运行；没有 runAsNonRoot、readOnlyRootFilesystem、drop capabilities；大量浮动标签（`latest`、用服务名当 tag、`alpine:latest`、`curlimages/curl:latest`）配合 `imagePullPolicy: Always`，部署不可复现；没有 PDB、affinity、topologySpread；没有 imagePullSecrets | 用非 root 用户；pin tag 或 digest；补 PDB 和反亲和 |
| R36 | 中 | 安全 | k8s-istio/istio_PeerAuthentication.yaml:5-16；istio_VirtualService.yaml:50-64；istio_AuthorizationPolicy.yaml:86-88；istio_opaGateway.yaml:50,58,95；istio_DestinationRule.yaml:189-212；istio_ServiceEntry.yaml:46-92 | 叠加样例自身就有风险：在 istio-system 设了网格级 STRICT mTLS，影响整个集群；/seckill 上常驻故障注入（0.1% 延迟 5s、0.1% 返回 400）；静态的 `x-api-key` 值直接写在策略里；CORS 全部放开；限流 EnvoyFilter 引用了没定义过的 `rate_limit_cluster`，CORS 用的是 Envoy v3 已废弃的字段（推断会被 NACK）；gateway 的 DR 设了 `tls: DISABLE`，和 STRICT 冲突；ServiceEntry 还指向阿里云 RDS MySQL | 明确标成"仅供演示"，或者按现在的架构重写后再叠加 |
| R37 | 中 | 性能 | deploy-coupon.yaml:18,30 | coupon 的 Deployment 和 Pod 注解用了 `now`，每次 helm upgrade（哪怕什么都没改）都会滚动重启秒杀热路径服务，渲染结果也不稳定。所有 Deployment 还同时写死了 `replicas`，会和 HPA 抢副本数 | 用配置 checksum 触发重启；开 HPA 时不要写 replicas |
| R38 | 中 | 一致性 | .github/workflows/ci.yml:27,39；qodana.yaml:26；.github/workflows/qodana_code_quality.yml:12-14；docs/CICD-SOURCE-OF-TRUTH.md「角色划分」 | CI 有覆盖缺口：`-pl` 里没有 connector-service；不跑 operator 测试、docker build、helm lint/template；Go 1.24 对 go.mod 的 1.23；Qodana 的 projectJDK 是 21，项目用的是 JDK 25；Qodana 监听 `dev` 分支，CI 监听的是 `develop` | 补齐这些检查，并加 helm 渲染校验 |
| R39 | 中 | 一致性 | Makefile:85,103-105,116,143,145-152,164,216；build.ps1:35,139；Jenkinsfile:112,149 | 三个构建入口互相漂移。Makefile 不构建 connector；依赖镜像在 Makefile 里是标签式，在 build.ps1 和 Jenkins 里是路径式；Makefile 的 canary 直接覆盖主 release，Jenkins 则另起一个 release；Makefile 的 kubebuilder 段引用了未定义的 `CONTROLLER_TOOL`、根目录的 `main.go` 和 `config/crd`，都跑不起来 | 以 Jenkins 为准重写 Makefile 和 build.ps1 的项目段；删掉失效的脚手架 |
| R40 | 中 | 文档漂移 | charts/couponkill/README.md（配置参数表、快速开始、"Go 默认关闭"、mTLS 描述）；docs/DEPLOYMENT-SOURCE-OF-TRUTH.md；docs/CICD-SOURCE-OF-TRUTH.md「变更纪律 3」 | README 里的默认值和 values 不符（go/coupon/order 副本写的是 2，实际是 1；nacos external 写的是 true，实际是 false；`image.tag` 实际不生效）。`helm repo add` 指向的是 GitHub 仓库，不是 Helm repo。README 说开 istio.enabled 就有 mTLS，但 Chart 里没有 PeerAuthentication。CICD 文档说镜像 tag 和 registry 在各入口之间是对齐的，事实上没有 | 以渲染结果为准改写文档 |
| R41 | 中 | 可维护性 | nacos-config-watcher.yaml:1-90；values.yaml:339-344 | 每 5 分钟跑一次的 CronJob 只会反复下载那个占位 URL。它把状态存在 Pod 自己的 /tmp 里，所以每次都会判断为"有更新"。要是 URL 真的配上了，它会每 5 分钟覆盖一次 Nacos 里的人工修改。容器也没设 resources | 默认关闭，或者直接删掉 |
| R42 | 中 | bug | templates/couponConfig.yaml:7 | 命名空间无条件打上 `istio-injection: enabled`。只要集群装了 Istio（哪怕 istio.enabled=false），hook Job 和 CronJob 都会被注入 sidecar；主容器退出后 sidecar 不退出，Job 就一直完不成，hook 卡住（推断） | 按 istio.enabled 决定是否打标签；Job 加 `sidecar.istio.io/inject: "false"` |
| R43 | 低 | 可维护性 | values-prod.yaml:1-18 等；couponkill-gateway/Dockerfile:1；couponkill-{user,coupon,order}-service/Dockerfile:1,16 | 中文注释已经损坏成 `????`（编码事故），可读性很差 | 恢复 UTF-8 注释 |
| R44 | 低 | 性能 | .dockerignore:1-2；Jenkinsfile:38-39 等 | 构建上下文是仓库根目录，只忽略了 .git，frontend 的 node_modules、logs、target 都会被发给 docker daemon；Java Dockerfile 没有依赖缓存层；Jenkins 对每个镜像用同一个 Dockerfile 构建两次，而不是 retag | 补全 .dockerignore；用 `--mount=type=cache`；改为 retag |
| R45 | 低 | 可维护性 | couponkill-gateway/Dockerfile:14,17（其他 Java Dockerfile 同理） | `-XX:+ZGenerational` 从 JDK 24 起已是 obsolete，JDK 25 启动时会打警告（不致命，已按 jdk25u 源码核实）。ENTRYPOINT 走 `sh -c` 且没有 exec，java 可能不是 PID 1，SIGTERM 不一定能转发到它（取决于 busybox 的行为，未验证） | 去掉这个 flag；改成 `exec java ...` |
| R46 | 低 | 死代码 | templates/configmap.yaml:1；values.yaml:320-324；values-prod.yaml:655-668,672-678,680-728；values.canary-keda.yaml:18-50（enabled 和 subset label）；charts/couponkill/scripts/init.sql.deprecated-mysql；04-seckill-reservation.sql | 大量 values 键没有模板引用（清单见 4.9）；configmap.yaml 永远不会渲染；MySQL 旧 DDL 和 04 迁移脚本没有任何执行入口 | 清理掉，或者接上线 |
| R47 | 低 | 可维护性 | deploy-order.yaml:28；各 Java Deployment 的探针 | order 只有 readiness，没有 liveness；liveness 打的是聚合的 `/actuator/health`，下游依赖出故障会连带重启；没有 startupProbe | 分别用 `/actuator/health/liveness` 和 `/readiness`，再加 startupProbe |
| R48 | 低 | 一致性 | deploy-gateway.yaml:59-60,67-71；deploy-user.yaml:79-82；deploy-order.yaml:31-36；pom.xml:62 | gateway 在生产环境也写死了 `SPRING_PROFILES_ACTIVE=dev`；`SPRING_REDIS_HOST/PORT` 是 Boot 2 的属性名（项目是 Boot 4.0.5，应为 `spring.data.redis.*`），默认 values 下 gateway 里渲染出来还是空值；order 的 `DB_USER`、`DB_PASS`、`KAFKA` 没有消费方（grep） | 删掉，或改成正确的属性名 |
| R49 | 低 | bug | sentinel.yaml:40；values.yaml:366-380；values-prod.yaml:448-449,455-458；Jenkinsfile 与 Makefile 的依赖镜像段 | 用默认 values 打开 Sentinel，模板直接报错（已核实）；prod 的 dashboard 镜像渲染成 `...my-docker:sentinel-dashboard:1.8.5`，有两个冒号，是非法引用（已核实）；版本 1.8.5 与镜像同步脚本推的 bladex 1.8.6 不一致；token-server 镜像 `sentinel-group/sentinel-token-server` 是否存在没有核实 | repository 和 tag 分开写对；统一版本 |
| R50 | 低 | 一致性 | nacos/DEFAULT_GROUP/couponkill-order-service-dev.yaml:17-18；nacos/shard/DEFAULT_GROUP/order-service-sharding.yaml:12,24；couponkill-coupon-service-dev.yaml:40-46；nacos/DEFAULT_GROUP/service-collaboration:5-7；nacos/SENTINEL_GROUP/couponkill-gateway-sentinel:1-29；nacos/DEFAULT_GROUP/gateway-routes.yaml:9-45 | Nacos 副本自身互相矛盾：注释说分片连接池是 300，实际配的是 32；coupon 的 `traffic.control.go-service.enabled` 和 service-collaboration 的 `fallback-to-go=true`，与"Go 热路径冻结"的说法相反（消费方未核实）；Sentinel 规则只覆盖 coupon、order、seckill-go，而网关路由上的 user-service 和 connector-service 没有规则 | 对照代码清理 |
| R51 | 低 | 死代码 | k8s-nothing/\*\*；cross-namespace-monitoring/rbac.yaml:39-68 | k8s-nothing 还是 MySQL/RocketMQ 时代的裸 Pod 加 hostPath PV（在 PV 上写 namespace 是无效的），另有明文口令。cross-namespace-monitoring 在 couponkill 命名空间里把 ClusterRole `prometheus-operator` 绑给了 operator 的 SA，而真正执行抓取的是 Prometheus 自己的 SA，这样绑定权限也过宽。这两个目录最近都只改过废弃横幅 | 移到 archive，或者删掉 |

## 7. 最近活跃度

`git log --oneline -n 20` 覆盖本分片全部路径，得到的 20 条提交全是 2026-07-18 这一天，是一次集中"收敛日"：

- PG/Kafka 迁移收尾：`f78b350` 删掉 Helm 里 rocketmq/mysql 的占位；`cb49c6d` 把 DB 集群开关的真源改为 `postgres.cluster`；`b9ce2a8` 让 values-prod 摆脱 MySQL 并清理秘密；`5ce5711` 让 nacos-init 的 fallback 对齐 PG/Kafka；`1d624eb` 调整依赖镜像拉取；`0e10e22` 加入 Kafka STS；`0203a78` 修改 Helm 的 Nacos env。
- 为了让 `helm template` 不再 nil panic 的补丁：`3a88dbe`（nacos checksum 默认值）、`eaaa424`（JVM opts 加引号）、`a8e9e85`、`67a0b9b`（canary 权重默认值）、`c9ddaab`（DR 默认值）。
- CI：`09e892e` 跳过需要中间件的 ApplicationTests；`3e34c79` 用 reactor 先构建 common。
- 文档：`4bcf536` 收敛部署和 CI 的真源文档，并给 DEPRECATED 目录加横幅；`eae3f1e` 更新 Chart 的中间件叙事。
- 业务功能顺带改动：`2dbcdee`、`30deb3b`（connector 比价）、`e7ee0a2`（按 requestId 轮询结果）、`747677d`（本地 PG 映射到 5433，加 JWT 冒烟）。

各路径的提交次数与最后一次变更：

| 路径 | 提交数 | 最后变更 |
|---|---|---|
| charts/couponkill/templates | 20 | 2026-07-18 `0e10e22` |
| charts/couponkill/values.yaml | 22 | 2026-07-18 `0e10e22` |
| charts/couponkill/values-prod.yaml | 10 | 2026-07-18 `f78b350` |
| charts/couponkill/values.canary-keda.yaml | 8 | 2026-07-18 `f78b350` |
| charts/couponkill/scripts | 5 | 2026-07-18 `0203a78` |
| nacos | 18 | 2026-07-18 `0203a78` |
| Jenkinsfile | 13 | 2026-07-18 `4bcf536` |
| Makefile / build.ps1 | 6 / 4 | 2026-07-18 `1d624eb` |
| .github/workflows/ci.yml / qodana | 6 / 2 | 2026-07-18 |
| docker-compose.migration.yml | 2 | 2026-07-18 `747677d` |
| Java Dockerfile（gateway、order 等） | 8 至 10 | 2026-07-13 `307f1ab` |
| couponkill-go-service/Dockerfile、couponkill-operator/Dockerfile | 9 / 3 | 2025-08-30 `06641ac`（已经很久没动） |
| k8s-istio | 4 | 2026-07-18 `2dbcdee` |
| k8s-nothing / cross-namespace-monitoring / ansible | 3 / 2 / 4 | 最近一次都只是 `4bcf536` 的文档横幅，实际已弃用 |

读法：整个仓库共 119 次提交（2025-08-07 至 2026-07-18）。Chart 最近的提交大多是为了让 lint/template 能过，没有哪一次提交能看出在真实集群上做过 install 验证。本地链路（compose + 导入脚本 + local-http-smoke）才是最近真正被验证的路径。Go 和 Operator 的 Dockerfile 停在 2025-08，和 Go 1.24 的 CI、Chart 里的端口约定都已经脱节。

## 8. 未验证 / 不确定项

- Helm 的运行期行为全部是推断，没有真正执行 `helm install`：同名对象会不会 AlreadyExists（R08）；`--create-namespace` 与 Chart 自带 Namespace 的所有权冲突（R06、R07）；hook 按 weight 和名字排序、逐个等待（3.2、R09）；Istio 注入 sidecar 后 Job 完不成（R42）。
- Nacos 3.1.1 在 K8s 里的行为：不配 `NACOS_AUTH_*` 能不能起来（R21，依据只是 compose 的注释）；`/nacos/v1/console/health/liveness` 在 3.x 上还在不在（nacos-init 等的就是这个接口）；Nacos 3 服务端 OpenAPI 默认是否开启鉴权。
- nacos-client 3.1.1 必须走 9848 gRPC（R12）：依据是 Nacos 2.x 以后客户端的通用行为，没有抓包验证。Go 用的 nacos SDK 版本和协议没有核实。
- Spring 的属性优先级（R13）：`SPRING_CLOUD_NACOS_SERVER_ADDR` 在显式配置了 `config.server-addr` 的情况下不生效，这是依据 SCA 的兜底逻辑推出来的，没有真的启动服务验证。
- 阿里云个人版 ACR 是否拒绝三级仓库路径（`my-docker/<svc>`），以及仓库是否公开、需不需要 imagePullSecrets（R04、R35）。
- apache/kafka 镜像默认的 `log.dirs` 路径（R33）。
- k8s-istio 的 EnvoyFilter 会不会被 Envoy NACK（R36）。
- `sh -c` 下 busybox ash 会不会对单条命令做 exec 优化（R45）。
- `sentinel-group/sentinel-token-server:1.8.5` 这个镜像是否存在。
- JVM 服务的真实启动耗时和探针 initialDelay（30 到 60 秒）是否匹配。
- Go 服务的环境变量契约是用 grep `os.Getenv` / `getEnvOrDefault` 得到的，如果代码里有其他读取方式（例如 flag 或第三方库）会漏掉，建议 go-operator 分片复核。
- 没有查看 Jenkins 和 GitHub Actions 的历史运行结果，所以不确定 CI 目前是绿是红。
- `ansible/` 和 `couponkill-operator/` 的内部实现没有深读。
- nacos-init 在占位 URL 返回 404 时，curl 加 unzip 的具体表现（Job 是 0 退出还是失败）没有实测；无论哪种，结果都是什么配置都没导入。

## 9. 候选 OpenSpec capability（只写现状）

说明：下面记录的是系统现在实际的行为，其中不少条目本身就是缺陷（见第 6 节）。落 spec 时建议先通过 change 提案修复，再把修复后的行为沉淀成 Requirement，避免把缺陷固化成需求。

### 9.1 `helm-chart-deployment`
目的：用一个 Helm release 把全部应用工作负载渲染进 `.Values.namespace`。
- Chart 为 gateway、user、coupon、order、connector、seckill-go、operator 各渲染一个 Deployment；除 coupon 外，每个服务再渲染一个"80 -> 容器端口"的 Service（templates/deploy-\*.yaml）。
- 镜像引用格式是 `<image.registry>:<services.<svc>.image.name>`，不使用 `image.tag`（deploy-gateway.yaml:31）。
- Java 工作负载通过 `JAVA_TOOL_OPTIONS`（默认 ZGC、CompactObjectHeaders、MaxRAMPercentage=75）和 `THREAD_POOL_OPTS`（`-Dspring.threads.virtual.enabled=true`）传入 JVM 参数（values.yaml:79-80 等；deploy-\*.yaml）。
- Chart 总会渲染带 `istio-injection: enabled` 的 Namespace，以及 default-deny 加两条 ingress 放行的 NetworkPolicy（couponConfig.yaml:1-12；network.yaml）。
- 只有在 `services.<svc>.hpa.enabled` 时才渲染 HPA，指标是 CPU 利用率 `hpa.cpu`，只适用于 gateway、user、order、go（deploy-\*.yaml 的 HPA 段）。

### 9.2 `helm-middleware-provisioning`
目的：在 Chart 里提供 Kafka、Nacos、Sentinel，以及初始化 hook。
- 当 `kafka.enabled && kafka.statefulSet.enabled`（默认如此）时，部署单节点 KRaft StatefulSet（apache/kafka:3.8.0，10Gi PVC），以及 `kafka`、`kafka-headless`、`broker` 三个 Service（kafka-statefulset.yaml）。
- Nacos 默认是单机 Deployment（nacos/nacos-server:v3.1.1，嵌入式存储，emptyDir）；`nacos.cluster.enabled` 时改为 StatefulSet 加 cluster.conf（nacos.yaml）。
- PostgreSQL 和 Redis 不部署，只渲染没有 selector 的 headless Service：postgres、postgres-master、postgres-slave、redis、redis-master、redis-slave（dependencies.yaml）。
- post-install/post-upgrade hook 包括：db-init（`db.init.enabled` 时执行 init-postgres.sql）、kafka-init（按 `kafka.topics` 和 `kafka.partitions` 建 topic）、nacos-init（导入外部 zip，或者把内联默认配置写入 public 命名空间）、redis-init（只做 ping）（\*-init-job.yaml）。

### 9.3 `nacos-config-distribution`
目的：约定应用配置在 Nacos 里的 dataId、group、namespace 布局，以及导入方式。
- Java 服务通过 `spring.config.import` 读取 `common.yaml` 和 `${spring.application.name}.yaml`（gateway 另外读 `gateway-routes.yaml`），group 为 DEFAULT_GROUP，命名空间为 `${NACOS_NAMESPACE:120}`（各服务 application.yml:6-23）。
- user、order、coupon 的 ShardingSphere 规则从 `<svc>-service-sharding.yaml` 读取，命名空间取 `shardingsphere.namespace`，值为 998（application.yml:22-25；\*ShardingSphereConfig.java）。
- Go 服务读取 `go-service-dev.yaml` / DEFAULT_GROUP，命名空间取 `NACOS_NAMESPACE_ID`，默认 120（config.go:178,187）。
- `scripts/import-nacos-local.ps1` 会在本地 Nacos 创建命名空间 120 和 998，发布 `nacos/DEFAULT_GROUP/*.yaml`、`couponkill-<svc>.yaml` 别名、SENTINEL_GROUP 规则和分片配置，并把容器主机名换成 127.0.0.1，端口分别为 5433、6379、9092、8848（import-nacos-local.ps1:25-83）。

### 9.4 `local-infra-compose`
目的：为本地联调提供迁移后的中间件。
- PostgreSQL 16 映射到宿主 5433，首次启动时依次执行 init-postgres.sql、02-seed-demo.sql、03-init-connector.sql（docker-compose.migration.yml:12-30）。
- Kafka 3.8.0 单节点 KRaft，容器内地址 `kafka:29092`，宿主地址 `localhost:9092`；kafka-init 以 16 分区、RF 1 创建 4 个 topic，其中包括 order_created（:39-82）。
- Redis 7 开启 AOF；Nacos v3.1.1 单机模式（Derby），暴露 8848 和 9848，关闭鉴权并提供 token 和 identity 占位值（:32-36,84-101）。

### 9.5 `postgres-schema-bootstrap`
目的：建库建表脚本。
- init-postgres.sql 以幂等方式创建 user_db_0/1、order_db_0/1、coupon_db_0/1，建出 "user"、user_coupon_count、order_0 至 order_15、coupon_0 至 coupon_15、stock_log_0 至 stock_log_15，并在 order_db_0 里额外建 seckill_reservation 和 user_notification（init-postgres.sql:16-300）。
- 03-init-connector.sql 创建 connector_db，以及 platform_sku_binding 和 coupon_price_map（03-init-connector.sql:4-41）。
- 04-seckill-reservation.sql 为存量库补上 seckill_start_at、seckill_end_at 两列，以及预约表和通知表，脚本是幂等的（04-seckill-reservation.sql:9-95）。

### 9.6 `pr-ci-checks`
目的：PR 与推送时的门禁。
- 向 develop 或 main 推送、或提 PR 时，用 Temurin JDK 25 执行 `mvn -B -pl couponkill-coupon-service,couponkill-order-service,couponkill-user-service,couponkill-gateway -am test`，排除 `*ApplicationTests`（ci.yml:5-30）。
- 同样的触发条件下，用 Go 1.24.x 在 couponkill-go-service 里执行 `go test ./...`（ci.yml:32-43）。
- 任意 PR 以及向 dev、main 推送时运行 Qodana（需要 `QODANA_TOKEN`），linter 为 qodana-jvm-community:2025.2（qodana_code_quality.yml:8-43；qodana.yaml:49）。
- CI 不构建镜像、不推送、不部署（ci.yml:1-3 的注释，文件里也没有相关步骤）。

### 9.7 `jenkins-cd-pipeline`
目的：构建镜像并用 Helm 发布。
- 并行构建 Java（`mvn clean package -DskipTests`）、Go 和 Operator（Jenkinsfile:14-32）。
- 为 7 个服务各构建两份镜像：`${REGISTRY}/<svc>:${BUILD_NUMBER}` 和 `${CANARY_REGISTRY}/<svc>:canary`，然后全部推送（Jenkinsfile:34-102）。
- 把 postgres:16、redis:7.0、nacos/nacos-server:v3.1.1、bladex/sentinel-dashboard:1.8.6、apache/kafka:3.8.0 重新打标，推到 `${REGISTRY}/<name>`（Jenkinsfile 的 Pull and Push Dependency Images 阶段，从 :104 附近开始）。
- 执行 `helm upgrade --install couponkill` 并带上 `--set image.tag=${BUILD_NUMBER}`，接着执行 `helm upgrade --install couponkill-canary -f values.canary-keda.yaml`；结束时用 Slack 通知结果（Jenkinsfile:137-165）。

### 9.8 `container-image-build`
目的：各模块镜像的构建方式。
- Java 镜像是两阶段构建：先用 maven（temurin-25）构建，再放到 eclipse-temurin:25-jre-alpine 运行。默认 `JAVA_TOOL_OPTIONS` 为 ZGC、ZGenerational、CompactObjectHeaders、MaxRAMPercentage=75（order 额外带 `--enable-preview`），入口是 `sh -c "java $THREAD_POOL_OPTS -jar app.jar"`（couponkill-\*/Dockerfile）。
- Go 镜像静态编译（CGO_ENABLED=0），以非 root 用户 seckill 运行，EXPOSE 8090，并带一个访问 /health 的 HEALTHCHECK（couponkill-go-service/Dockerfile:18-54）。
- Operator 镜像是 amd64 静态二进制，运行在 distroless/static:nonroot 上（UID 65532）（couponkill-operator/Dockerfile:20-38）。
- 所有镜像都以仓库根目录为构建上下文（`-f <module>/Dockerfile .`），`.dockerignore` 只排除了 `.git`（Makefile:147 等；.dockerignore:1-2）。

### 9.9 `istio-traffic-policy`
目的：`istio.enabled` 时的入口与流量策略。
- 渲染 Gateway（selector 为 `istio: ingressgateway`，HTTP 80，host 取 `istio.gateway.host`）和 VirtualService couponkill-vs，按前缀把 /seckill、/api/v1/user、/api/v1/order、/api/v1/coupon、/nacos、/sentinel 路由到对应的 Service，并带重试和超时（istio.yaml:6-138）。
- 为 gateway、go、user、order、coupon、nacos、sentinel 各渲染一个 DestinationRule，包含连接池、outlierDetection、ISTIO_MUTUAL 和 LEAST_REQUEST（istio.yaml:143-330）。
- coupon 和 order 另外有 stable/canary 子集的 DR，以及一个同名 VirtualService，按 `canary.*.weightStable` 和 `weightCanary` 分流（destinationrules.yaml；virtualservice-canary.yaml）。

### 9.10 `kafka-lag-autoscaling` 与 `metrics-scrape-config`（可以合并进 9.1）
- 当 `keda.enabled`，且 `keda.kafka.goEdge.enabled` 或 `keda.kafka.order.enabled` 为真时，渲染 ScaledObject：kafka 触发器，带 bootstrapServers、consumerGroup、topic、lagThreshold，pollingInterval 15、cooldown 60；同时渲染 TriggerAuthentication `kafka-trigger-auth`（keda.yaml:1-66）。
- Java Pod 带注解 `prometheus.io/path=/actuator/prometheus`，Go Pod 带 `/metrics`；当 `monitoring.enabled && monitoring.prometheus.enabled` 时，渲染 go、user、order、coupon 的 ServiceMonitor 和 Nacos 的 PodMonitor（monitoring.yaml:1-87）。

## 10. 结论与建议（按修复顺序，只给方向，不实施）

1. 先让镜像能构建出来（R01、R05、R44）。把 builder 换成存在的 `maven:3.9.11-eclipse-temurin-25`（或更新）并 pin digest；给 operator 目录配一份 Makefile，或者在 Jenkins 里改掉这一步；CI 里至少加一次 `docker build`（一个 Java 镜像加 Go 镜像）。
2. 让 Chart 能装上（R02、R03、R06、R08、R09、R42）。修掉 `coupon}`；补上 coupon 的 Service 和 HPA；从 Chart 里去掉 Namespace，或者去掉 `--create-namespace`；删掉 dependencies.yaml 里的 nacos 和 sentinel 占位；两份 VirtualService 合并；kafka-init 改用 bash 加 `/opt/kafka/bin/kafka-topics.sh`；hook 加 weight，并禁止 sidecar 注入。把 `helm lint`、`helm template | kubeconform` 和一个简单的"重复对象检查"放进 CI。
3. 打通 Nacos（R10 至 R13、R20、R21）。只留一份配置真源：把 `nacos/` 放进 chart 可读的目录，由导入 Job 按 120/998 发布，或者整体统一成一个命名空间约定；Service 补 9848；给 Nacos 配 PVC 和 AUTH 变量；所有 Java 服务统一注入 `NACOS_SERVER_ADDR`、`NACOS_NAMESPACE`、`SERVER_PORT`。
4. 统一镜像命名（R04、R07、R39）。选定一种（个人版 ACR 建议标签式，tag 用 `<svc>-<build>`），让模板真正读 tag，Jenkins、Makefile、build.ps1 三处都按这一种来；canary 在同一个 release 里用独立的 canary Deployment 实现，不要再起第二个 release。
5. 安全（R16、R17、R18、R22、R34、R35）。Istio 入口只指向 couponkill-gateway，或者加 RequestAuthentication 并在边缘剥掉 `X-User-*`；删除 `/nacos`、`/sentinel` 公网路由和 `nacos-external` LoadBalancer；立刻轮换 k8s-nothing 里泄露的 ACR 凭据，并清理 git 历史；密码和 JWT secret 迁到 K8s Secret（或外部密钥管理），Chart 负责声明并写进文档；Java 镜像改用非 root 用户。
6. 网络与弹性（R15、R24、R27、R28、R30、R37）。NetworkPolicy 补 egress 放行和正确的命名空间标签；统一 HPA 的键名；KEDA 的 topic、消费组、Deployment 名以代码为准；金丝雀标签体系重新设计；补上 Prometheus registry 并给端口命名；去掉 `now` 注解。
7. 契约清理（R23、R25、R26、R31、R32、R33、R46、R48）。修掉 `default` 吞 false 的问题；去掉 operator 探针参数上的 quote；CRD 只从 operator 生成，并放进 `crds/`；Go 的 env 以代码为准；db-init 补执行 03 脚本；Kafka 设置 `KAFKA_LOG_DIRS`；清理死键和死模板。
8. 文档（R14、R38、R40）。以渲染结果为准改写 DEPLOYMENT/CICD 真源文档和 Chart README，写明 PG/Redis 外置、所需 Secret 和命名空间约定；CI 的 `-pl` 加上 connector，Go 和 Qodana 的 JDK 版本与项目对齐。

对 OpenSpec 的建议：第 9 节的 capability 可以直接作为 `openspec/specs/` 的初始清单。但 9.1、9.2、9.3、9.7 的现状里包含不少缺陷，更稳妥的做法是先起一个类似 `fix-helm-deployment-contract` 的 change，把第 1 至 4 步落成 proposal 和 tasks，修复之后再归档成 spec。

