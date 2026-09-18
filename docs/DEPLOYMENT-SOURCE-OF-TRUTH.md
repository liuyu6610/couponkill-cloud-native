# 部署真源（Source of Truth）

> 生效日期：2026-07-18  
> 修订：2026-09-18（记录 [#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 合并后的轮换项与剩余债）  
> 目的：消除多套部署入口并存造成的漂移，明确「生产以谁为准」。

## 唯一生产入口

| 角色 | 路径 | 说明 |
|------|------|------|
| **生产 / 演示集群部署真源** | [`charts/couponkill`](../charts/couponkill) | Helm Chart：服务、中间件、Nacos init、Istio 基础开关、KEDA 等 |
| Schema 真源 | [`charts/couponkill/scripts/init-postgres.sql`](../charts/couponkill/scripts/init-postgres.sql) | PG 建库建表 |
| 本地中间件联调 | [`docker-compose.migration.yml`](../docker-compose.migration.yml) | 仅本地 PG(5433)/Redis/Kafka/Nacos，**不是** K8s 生产入口 |
| 本地 Nacos 导入 | [`scripts/import-nacos-local.ps1`](../scripts/import-nacos-local.ps1) | 把仓库 `nacos/` 灌进本地 Nacos（PG→127.0.0.1:5433） |
| 本地 HTTP 冒烟 | [`scripts/local-http-smoke.ps1`](../scripts/local-http-smoke.ps1) | 起 Java 服务做 JWT + 契约冒烟 |
| CI/CD 真源说明 | [`CICD-SOURCE-OF-TRUTH.md`](./CICD-SOURCE-OF-TRUTH.md) | Jenkins=CD；GHA=PR 校验 |

## 可选 / 非真源路径

| 路径 | 状态 | 用途 |
|------|------|------|
| `couponkill-operator/` | **可选** | CRD/Operator 编排；默认演示仍以 Helm 为准，Operator 不替代 Chart 真源 |
| `k8s-istio/` | **补充样例（非独立入口）** | 高级 Istio 清单；须在 Helm `istio.enabled` 之上叠加，禁止单独当整套部署 |
| `k8s-nothing/` | **DEPRECATED** | 历史简易一体包（仍含 MySQL/RocketMQ 叙事），禁止用于新环境 |
| `ansible/` | **DEPRECATED** | 历史运维剧本，禁止作为新部署入口 |
| `cross-namespace-monitoring/` | **DEPRECATED** | 实验性跨命名空间监控样例，非生产基线 |

## 变更纪律

1. 生产清单变更：只改 `charts/couponkill`（及必要的 Nacos 仓库副本 `nacos/`），再同步文档。  
2. 禁止在 DEPRECATED 目录上叠加新功能；若需保留样例，仅允许修文档标注。  
3. 本地联调与生产 Chart 配置漂移时，以「能跑通 `local-http-smoke.ps1` + Chart values 可解释」为准收敛。

## 凭证注入（生产）

禁止把真实口令、Token、dockerconfigjson 提交进 Git。Chart 侧命令见 [`charts/couponkill/README.md`](../charts/couponkill/README.md)。

1. 复制根目录 [`.env.example`](../.env.example) 为 `.env`（已 gitignore），仅用于本地 compose。
2. 集群使用 Kubernetes Secret（默认名 `couponkill-app-secrets`），由 Deployment `secretKeyRef` 注入 `POSTGRES_PASSWORD` / `JWT_SECRET` / `CONNECTOR_INTERNAL_TOKEN`。
3. 生产 Helm：`secrets.create=false` + 预先创建 `secrets.existingSecret`，或 External Secrets Operator。
4. 若仓库历史中出现过真实密钥，必须在对应平台**轮换**，不能只删文件（见下一节）。

### #4 合并后仍须人工轮换

[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 已从工作区删除或改为占位符，**Git 历史仍可能残留**。下表**不写密钥正文**；审查历史 diff 时也不要展开相关删除。

| 项 | 历史位置（工作区已出库） | 须在平台侧做的事 |
| --- | --- | --- |
| Apifox Token | `.idea/ApifoxUploaderProjectSetting.xml`（`apiAccessToken`） | 在 Apifox 控制台吊销/轮换该个人访问令牌 |
| ACR `dockerconfigjson` | `k8s-nothing/couponkill-simple/couponKill.yaml`（镜像拉取 Secret） | 在阿里云 ACR 轮换密码/Token；检查该 Secret 是否曾落到集群 |
| Git 历史残留主机名 | `.idea/dataSources.xml`（RDS / Redis 公网主机名） | 收紧 RDS/Redis 访问白名单；主机名仍可从 Git 历史读出 |

生产集群另须：用强随机值创建/替换 `couponkill-app-secrets`，不要沿用演示口令。本地 compose：复制 `.env.example` → `.env` 后再覆盖 Nacos/JWT。

### 已知剩余债（文档记录，本轮不修）

| 项 | 说明 | 范围 |
| --- | --- | --- |
| Java 业务镜像仍可能以 root 启动 | 当前 Java Dockerfile 无 `USER`；Chart 不默认 `runAsNonRoot`，以免 Pod 无法调度。Go / Operator 镜像已非 root。 | 后续改镜像后再收紧 Pod |
| `templates/nacos-init-job.yaml` 既有 YAML 问题 | Helm 渲染时部分块缩进会导致 parse 失败（#4 改前即存在，未整文件重排）。 | 非本次文档范围 |
| 开放 PR [#1](https://github.com/liuyu6610/couponkill-cloud-native/pull/1)、[#3](https://github.com/liuyu6610/couponkill-cloud-native/pull/3) | `#1` dockerfile complete；`#3` Qodana CI。 | 非本次范围 |

