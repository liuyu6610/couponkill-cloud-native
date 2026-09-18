# 安全运维：P0 密钥轮换、生产 Secret 注入与残留风险

> 背景：[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 已把工作区明文凭证出库并收紧 Helm 默认；[#5](https://github.com/liuyu6610/couponkill-cloud-native/pull/5) 在部署真源记了摘要。  
> 本页是 **运维可执行清单**（文档 only，不改 Chart / 业务代码）。  
> **禁止**把真实口令、Token、`dockerconfigjson`、白名单 IP、主机名正文写进 Git、Issue、PR 评论或本文件。审查历史 diff 时不要展开已删除的密钥文件。

部署路径仍以 [`DEPLOYMENT-SOURCE-OF-TRUTH.md`](./DEPLOYMENT-SOURCE-OF-TRUTH.md) 为准；Chart 命令入口见 [`charts/couponkill/README.md`](../charts/couponkill/README.md)。

---

## 1. P0 密钥轮换清单

[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 只清理了**当前工作区**。下列项一旦曾出现在 Git 历史中，**删文件不等于失效**，必须在对应平台轮换。本表不写密钥正文。

| 优先级 | 项 | 历史位置（工作区已出库 / 已改占位符） | 平台侧动作 | 完成判据 |
|--------|----|----------------------------------------|------------|----------|
| **P0** | Apifox Token | `.idea/ApifoxUploaderProjectSetting.xml`（`apiAccessToken`，个人访问令牌形态） | 登录 Apifox 控制台，**吊销**旧 Token 并重新签发。新 Token 只放本机 IDE / CI 密钥库，已加入 `.gitignore`，禁止再提交 | 旧 Token 调用失败；仓库与 PR 评论中无 Token 正文 |
| **P0** | 阿里云 ACR 拉取凭证 | `k8s-nothing/couponkill-simple/couponKill.yaml`（`kubernetes.io/dockerconfigjson`） | 在阿里云容器镜像服务轮换该仓库的密码 / 临时 Token；检查曾应用过 `k8s-nothing` 的集群是否仍有旧 `docker-registry` Secret，有则删除后按下方命令重建 | 旧口令无法 `docker login` / 拉镜像；集群内旧 Secret 已替换 |
| **P0** | RDS / Redis 白名单 | `.idea/dataSources.xml`（RDS MySQL / Redis **公网主机名**指纹） | 在阿里云 RDS、Redis 控制台收紧**访问白名单 / 安全组**：仅允许已知办公网或集群 NAT 出口，去掉 `0.0.0.0/0` 或过宽网段 | 未知公网源连不上；主机名仍可能从 Git 历史读出，故白名单是硬控制 |

工作区防护（已在 [#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 落地，运维只需确认未被改回）：

- `.gitignore` 已忽略 `.idea/ApifoxUploaderProjectSetting.xml`、`.idea/dataSources.xml` 与 `.env`
- `k8s-nothing/` 为 **DEPRECATED**，禁止用于新环境；其中镜像拉取 Secret 已改为占位符

ACR 镜像拉取 Secret（占位，勿填真实口令进仓库）：

```bash
kubectl -n couponkill create secret docker-registry acr-pull \
  --docker-server='<ACR 仓库域名>' \
  --docker-username='<只读拉取账号>' \
  --docker-password='<轮换后的 Token>'
```

---

## 2. 运维说明：生产安装前创建 `couponkill-app-secrets`

[`charts/couponkill/values-prod.yaml`](../charts/couponkill/values-prod.yaml) 已固定：

```yaml
secrets:
  create: false
  existingSecret: couponkill-app-secrets
  name: couponkill-app-secrets
```

含义：Chart **不会**创建 Secret。`helm upgrade --install … -f values-prod.yaml` 之前，目标命名空间里必须已有同名 Secret，否则业务 Pod 的 `secretKeyRef` 无法解析（常见现象：`CreateContainerConfigError`）。

演示默认 [`values.yaml`](../charts/couponkill/values.yaml) 才是 `secrets.create: true`（写入本地演示口令 `postgres`，JWT 等首次 install 随机生成）。**生产禁止**用演示 values 直接装。

### 2.1 必须有的 key

默认 key 名见 `secrets.keys`（[`charts/couponkill/templates/_helpers.tpl`](../charts/couponkill/templates/_helpers.tpl)）：

| Secret key | 注入的环境变量 | 用途 |
|------------|----------------|------|
| `postgres-password` | `POSTGRES_PASSWORD`、`SPRING_DATASOURCE_PASSWORD` | PG 口令；Nacos / ShardingSphere YAML 里的 `${POSTGRES_PASSWORD}` 由进程展开 |
| `jwt-secret` | `JWT_SECRET` | HMAC；对应配置里的 `${JWT_SECRET}` |
| `internal-token` | `CONNECTOR_INTERNAL_TOKEN` | Connector 内部调用 |
| `nacos-auth-token` | Nacos 容器 `NACOS_AUTH_TOKEN` | Nacos 3.x 占位鉴权。写入 Secret 的值应是：**原文 ≥32 字符后再 Base64**（`kubectl --from-literal` 还会再做一次 Kubernetes 的 Base64 编码，这是预期行为） |

不要把上述真实值写进 `values-prod.yaml` 或 Git。

### 2.2 推荐顺序

```bash
# 1. 命名空间（Secret 必须落在 Chart 使用的 namespace）
kubectl create namespace couponkill --dry-run=client -o yaml | kubectl apply -f -

# 2. 预创建凭证（把尖括号换成本机生成的随机值；不要记录到仓库）
kubectl -n couponkill create secret generic couponkill-app-secrets \
  --from-literal=postgres-password='<强随机>' \
  --from-literal=jwt-secret='<≥32字节随机>' \
  --from-literal=internal-token='<强随机>' \
  --from-literal=nacos-auth-token='<原文≥32字符后再 Base64>'

# 3. 确认 key 存在（不要 get -o yaml 把 data 贴到聊天或 PR）
kubectl -n couponkill get secret couponkill-app-secrets \
  -o jsonpath='{range $k,$v := .data}{$k}{"\n"}{end}'

# 4. 再装生产 overlay（仓库根目录）
helm upgrade --install couponkill ./charts/couponkill \
  --namespace couponkill \
  -f ./charts/couponkill/values-prod.yaml
```

等价入口：`make deploy-chart-prod`（内部同样 `-f values-prod.yaml`）。若跳过第 2 步直接 make，Pod 会因缺 Secret 起不来。

更新已有 Secret（轮换口令后需滚动工作负载，使新 env 生效）：

```bash
kubectl -n couponkill create secret generic couponkill-app-secrets \
  --from-literal=postgres-password='<新值>' \
  --from-literal=jwt-secret='<新值>' \
  --from-literal=internal-token='<新值>' \
  --from-literal=nacos-auth-token='<新值>' \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n couponkill rollout restart deploy
```

### 2.3 其它注入方式

只要最终对象是同命名空间、同名、含上表四个 key 的 `Secret`，Chart 不关心来源：

- External Secrets Operator / 云厂商 Secret Store CSI：把外部密钥映射到 `couponkill-app-secrets`
- SealedSecret / SOPS：解密结果仍须是该名字

禁止：把口令写进 Deployment `env.value`、Nacos 明文 content、或 `values-prod.yaml` 的 `secrets.postgresPassword`。

### 2.4 本地 compose（不是生产）

复制根目录 [`.env.example`](../.env.example) 为 `.env`（已 gitignore）后再覆盖占位值。compose 无 `.env` 时可能落回演示占位，仅限本机。

---

## 3. 残留风险（仅文档，本轮不改代码）

[#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) 明确未修、后续另开专项。此处只登记，避免被当成「已经安全」。

| 项 | 现状 | 为何本轮只记文档 | 后续方向（不在本 PR） |
|----|------|------------------|------------------------|
| **Java 镜像仍以 root 启动** | `couponkill-gateway` / `user` / `coupon` / `order` / `connector` 的 Dockerfile **无 `USER`**。Chart 对 Java 容器只 `drop ALL` + `allowPrivilegeEscalation: false`，**不**默认 `runAsNonRoot`。Go（`USER seckill`）与 Operator（`USER 65532`）已非 root，对应 Deployment 另有 `runAsNonRoot: true` | 强开 `runAsNonRoot` 会因镜像仍是 root 而无法调度 | 先给 Java 镜像加非 root 用户，再收紧 Pod `securityContext` |
| **Istio `tls: false`** | [`values-prod.yaml`](../charts/couponkill/values-prod.yaml) 与 [`values.canary-keda.yaml`](../charts/couponkill/values.canary-keda.yaml) 均为 `istio.gateway.tls: false`（明文 HTTP）。Chart [`templates/istio.yaml`](../charts/couponkill/templates/istio.yaml) 入口 Gateway 当前按 **HTTP :80** 渲染，未消费该开关。`k8s-istio/istio_gateway.yaml` 虽有 443 + `credentialName`，只是叠加样例，不是生产真源 | 仓库没有证书 / cert-manager 来源；改 TLS 会打断现有 HTTP 入口 | 选定证书来源后改 Gateway 为 HTTPS，并同步 DNS / 探活 |
| **`latest` 标签** | 演示默认 [`values.yaml`](../charts/couponkill/values.yaml) `image.tag: latest`；Nacos configWatcher 使用 `curlimages/curl:latest`；本地 `Makefile` / `build.ps1` 仍打 `:latest`。生产 overlay 业务镜像为 `image.tag: "v1.0.0"`，**仍非 digest 钉死** | 钉版本 / digest 依赖发布流水线与镜像仓库约定，改 tag 会牵动 Jenkins CD | Jenkins 发布改用构建号或 sha256 digest；演示 values 与 watcher 镜像去掉浮动 `latest` |

其它 [#4](https://github.com/liuyu6610/couponkill-cloud-native/pull/4) / [#5](https://github.com/liuyu6610/couponkill-cloud-native/pull/5) 已登记、仍非本页范围的债：`templates/nacos-init-job.yaml` 既有 YAML 缩进导致部分 Helm 解析失败；开放 PR [#1](https://github.com/liuyu6610/couponkill-cloud-native/pull/1)、[#3](https://github.com/liuyu6610/couponkill-cloud-native/pull/3)。
