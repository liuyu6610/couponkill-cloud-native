# 00 · OpenSpec explore 索引（2026-10-01）

> 仓库：`couponkill-cloud-native`。基线：`main` @ `3969213`（2026-07-18，本轮已用 `git rev-parse HEAD` 核对）。
> 模式：只读探查。本目录只放报告，不改产品代码，不建 `openspec/specs` capability，不提交。
> 标注沿用各分片：【已核实】读代码或命令输出；【推断】未运行验证。

## 分片状态

| 文件 | 范围 | 状态 |
|---|---|---|
| `00-index.md` | 本索引 | 完成 |
| `01-java-foundation.md` | 根 POM、`couponkill-common`、gateway、user-service | 完成（本轮重写，替换「待补充」骨架） |
| `02-java-business.md` | coupon / order / connector | 完成 |
| `03-go-operator.md` | `couponkill-operator/`（实际目录，不是另起的模块名） | 完成 |
| `04-frontend.md` | 已提交的 `frontend/couponkill-frontend`；未跟踪的 `frontend/couponkill-miniprogram` | 完成 |
| `05-deploy-cicd.md` | Chart、compose、Jenkins、GitHub Actions、Nacos 副本 | 完成（写到第 10 节） |
| `06-ops-tooling.md` | `scripts/`、`ansible/`、`jmeter/`、`linux-shell-sh/`、`test-python/` | 完成 |
| `07a-crosscut-flows.md` | 端到端链路、文档对照、鉴权链 | 完成 |
| `07b-crosscut-consistency.md` | 端口、Nacos dataId、Feign、MQ、Redis、DB、错误体 | 完成 |

## 跨分片风险（先看这些）

1. 身份在几条路径上都可以伪造。07a 已核实：`POST /api/v1/auth/token/mock` 按请求体签发任意 `roles`，且在网关白名单。05 R16 已核实：Istio 入口不经过网关，下游只认 `X-User-Id` / `X-User-Roles`。01 补了一条：一旦配置了 `CONNECTOR_ADMIN_TOKEN`，管理令牌放行时不覆盖客户端带来的 `X-User-Id`。
2. 秒杀主链路的库存记账和订单状态在 02 的 TL;DR 里已有四条高优先级缺陷（全分片误扣、成功单被写成已使用、Redis 预扣不守恒、停用券挡不住下单）。07a 把这四条接到登录后的路径上，记账细节仍以 02 第 6 节为准。
3. 文档上的生产真源是 Helm + Jenkins，05 的结论是构建镜像、Chart 安装、Nacos 内容和入口安全对不上代码。本地 compose 导入是目前唯一和 Java 配置同名的配置链路。
4. `couponkill-operator` 是第二套部署器：只创建不更新、Pod 标签是短名 `app=coupon` 这一类、示例环境变量对不上 Java/Go 的真实配置；Chart 里还有另一份 CRD（05 R26）。它没有接管 Helm 的 Deployment。
5. 用户名在每个分片库里单独 UNIQUE，登录 SQL 不带分片键（01）。Web 登录把口令放在 query string（01 与 04）。分片 YAML 热更新会关闭仍被 Spring 持有的数据源：coupon/order 见 02 第 5 点，user-service 是同一段写法（01）。
6. 已提交前端没有 CI 和镜像，生产 `VITE_API_BASE_URL` 为空（04）。小程序已从 git 删除（`afddf63`），工作区里的副本没有 `app.json`，不能当作可运行客户端。
