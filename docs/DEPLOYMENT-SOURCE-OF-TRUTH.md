# 部署真源（Source of Truth）

> 生效日期：2026-07-18  
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
