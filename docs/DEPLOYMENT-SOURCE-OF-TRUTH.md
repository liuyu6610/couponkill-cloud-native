# 部署真源（Source of Truth）

> 生效日期：2026-07-18  
> 修订：2026-09-18（[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 出库后的轮换 / 注入 / 残留风险见 [`SECURITY-ROTATION.md`](./SECURITY-ROTATION.md)）  
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
| 密钥轮换 / 生产 Secret / 残留风险 | [`SECURITY-ROTATION.md`](./SECURITY-ROTATION.md) | P0 轮换清单；`couponkill-app-secrets` 注入；Java root / Istio `tls:false` / `latest` |

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

禁止把真实口令、Token、dockerconfigjson 提交进 Git。

**完整 P0 轮换清单、生产安装前如何创建 `couponkill-app-secrets`、以及残留风险（Java root / Istio `tls:false` / `latest`）见 [`SECURITY-ROTATION.md`](./SECURITY-ROTATION.md)。** Chart 侧命令摘要见 [`charts/couponkill/README.md`](../charts/couponkill/README.md)。

1. 复制根目录 [`.env.example`](../.env.example) 为 `.env`（已 gitignore），仅用于本地 compose。
2. 集群使用 Kubernetes Secret（默认名 `couponkill-app-secrets`），由 Deployment `secretKeyRef` 注入 `POSTGRES_PASSWORD` / `JWT_SECRET` / `CONNECTOR_INTERNAL_TOKEN`。
3. 生产 Helm：`values-prod.yaml` 已 `secrets.create=false`，**必须先**预创建 `secrets.existingSecret`（或 External Secrets 写入同名对象），再 `helm upgrade --install`。
4. 若仓库历史中出现过真实密钥，必须在对应平台**轮换**，不能只删文件。

### P0 人工轮换（摘要）

[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 已从工作区删除或改为占位符，**Git 历史仍可能残留**。下表**不写密钥正文**。步骤与完成判据以 [`SECURITY-ROTATION.md`](./SECURITY-ROTATION.md) 为准。

| 优先级 | 项 | 历史位置（工作区已出库） | 须在平台侧做的事 |
| --- | --- | --- | --- |
| P0 | Apifox Token | `.idea/ApifoxUploaderProjectSetting.xml` | 在 Apifox 控制台吊销/轮换该个人访问令牌 |
| P0 | 阿里云 ACR 拉取凭证 | `k8s-nothing/couponkill-simple/couponKill.yaml` | 在 ACR 轮换密码/Token；检查该 Secret 是否曾落到集群 |
| P0 | RDS / Redis 白名单 | `.idea/dataSources.xml`（公网主机名指纹） | 收紧 RDS/Redis 访问白名单；主机名仍可从 Git 历史读出 |

### 已知剩余债（文档记录，本轮不修代码）

| 项 | 说明 | 范围 |
| --- | --- | --- |
| Java 业务镜像仍以 root 启动 | Java Dockerfile 无 `USER`；Chart 不默认 `runAsNonRoot`，以免 Pod 无法调度。Go / Operator 已非 root。 | 后续改镜像后再收紧 Pod |
| Istio `tls: false` | 生产 / 金丝雀 overlay 入口为明文 HTTP；Chart Gateway 当前按 :80 渲染。缺证书来源。 | 有证书后再改 HTTPS |
| 应用 / 工具镜像 `latest` | 演示 `values.yaml` 为 `image.tag: latest`；configWatcher 用 `curl:latest`。生产 overlay 业务 tag 为 `v1.0.0`，仍非 digest。 | 依赖发布流水线钉版本 |
| `templates/nacos-init-job.yaml` 既有 YAML 问题 | Helm 渲染时部分块缩进会导致 parse 失败（#4 改前即存在，未整文件重排）。 | 非本次文档范围 |
| 开放 PR [#1](https://github.com/liuyu6610/couponkill-cloud-native/pull/1)、[#3](https://github.com/liuyu6610/couponkill-cloud-native/pull/3) | `#1` dockerfile complete；`#3` Qodana CI。 | 非本次范围 |

