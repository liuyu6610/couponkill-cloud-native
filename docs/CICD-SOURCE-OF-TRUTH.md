# CI/CD 真源（Source of Truth）

> 生效日期：2026-07-18  
> 目的：消除「Jenkins / GitHub Actions / 文档叙事」多套流水线并存造成的漂移，明确构建与发布以谁为准。  
> 部署制品真源仍见 [`DEPLOYMENT-SOURCE-OF-TRUTH.md`](./DEPLOYMENT-SOURCE-OF-TRUTH.md)（Helm `charts/couponkill`）。

## 角色划分

| 角色 | 路径 | 说明 |
|------|------|------|
| **生产 / 演示集群 CD 真源** | [`Jenkinsfile`](../Jenkinsfile) | 构建 Java/Go/Operator → 构建并推送镜像 → `helm upgrade --install` 部署 `charts/couponkill`（含金丝雀 values） |
| **镜像本地构建入口** | [`Makefile`](../Makefile) / [`build.ps1`](../build.ps1) | 开发机构建/打 tag；**不替代** Jenkins 发布职责 |
| **GitHub PR / push 校验（CI）** | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) | 父工程 `-am` 编译 + 单元/契约测试（排除 `*ApplicationTests`）；**不**起中间件、**不**推镜像、**不**改集群；完整上下文冒烟用 `scripts/local-http-smoke.ps1` |
| **可选质量扫描** | [`.github/workflows/qodana_code_quality.yml`](../.github/workflows/qodana_code_quality.yml) | Qodana；需 `QODANA_TOKEN`，失败不替代 Jenkins CD |
| 本地 HTTP 冒烟 | [`scripts/local-http-smoke.ps1`](../scripts/local-http-smoke.ps1) | 开发机联调验证，不属于集群 CD |

## 明确非真源 / 禁止叙事

| 说法或路径 | 状态 |
|------------|------|
| 「GitHub Actions 推 main 触发 Argo CD」 | **无效叙事** — 仓库无 Argo Application、无 `bump-chart-version.sh`；禁止再写进文档或 workflow |
| 「仅靠 GHA 完成生产部署」 | **禁止** — 生产/演示 CD 以 Jenkins + Helm 为准 |
| `ansible/` 内嵌 CI | **DEPRECATED**（随 ansible 部署路径一并废弃） |

## 推荐流水线语义

```
开发者 PR
  → GitHub Actions ci.yml（test / compile）
  → （可选）Qodana

合并到可发布分支并人工/定时触发 Jenkins
  → Jenkinsfile：package → docker build/push → helm upgrade charts/couponkill
  → 集群状态以 Helm release 为准
```

## 变更纪律

1. 改集群发布步骤：只改 `Jenkinsfile`（及必要的 Chart values），再同步本文件与根 README「CI/CD」节。  
2. 改 PR 门禁：只改 `.github/workflows/ci.yml`；不得在该文件里加入「假装已部署」的步骤。  
3. 镜像 tag / registry 变量以 Jenkins `environment` 与 Makefile/`build.ps1` 对齐；漂移时以 **Jenkinsfile 环境变量** 为发布侧真源。  
4. 禁止新增第二套会 `helm upgrade` 生产命名空间的并行流水线，除非先更新本文件并明确切换窗口。
