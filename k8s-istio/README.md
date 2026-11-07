# CouponKill Istio 叠加配置

> **非独立部署入口。** 部署真源是 [`charts/couponkill`](../charts/couponkill)：开启 `istio.enabled=true` 后 chart 自己渲染入口
> `Gateway`、入口 `VirtualService`、每个服务一条 `DestinationRule`（mTLS + 连接池 + 熔断 + stable/canary 子集）和金丝雀 `VirtualService`。
> 本目录只放 chart 不管的那部分网格安全与可观测配置，在 chart 部署之后叠加。详见
> [`docs/DEPLOYMENT-SOURCE-OF-TRUTH.md`](../docs/DEPLOYMENT-SOURCE-OF-TRUTH.md)。

## 暴露面：只有 gateway

```
外部 → Istio 入口网关 ──(/api/、/healthz)──→ couponkill-gateway ──→ user / coupon / order / connector
                           其余路径 404                  （JWT 校验、身份头 HMAC 签名、/internal/** 一律 404）
```

* 入口 `VirtualService`（chart 渲染）只放行 `istio.virtualService.exposePrefixes`（默认 `/api/`、`/healthz`），其余直接 404，
  所以 `/metrics`、`/readyz`、`/internal/**` 都出不去。
* user / coupon / order / connector 没有任何入口路由。
* 即使有人绕过网关直连业务服务（比如 Istio 配置失误），它们也只信带有效 `X-Gateway-Sig` 的身份头，
  `/internal/**` 还要 `X-Internal-Token`（设计文档 §5）。下面的授权策略是网格层面的第三道闸。

## 文件

| 文件 | 作用 |
|------|------|
| `istio_config.yaml` | `couponkill` 命名空间与 `istio-injection=enabled` 标签 |
| `istio_PeerAuthentication.yaml` | 命名空间内 STRICT 双向 TLS |
| `istio_AuthorizationPolicy.yaml` | 五个服务账户的授权策略（见下表），外加「gateway 不得访问 `/internal/**`」的 DENY 与 Prometheus 抓取放行 |
| `istio_Sidecar.yaml` | 限制代理的出站可见范围（同命名空间 + `istio-system`） |
| `istio_Telemetry.yaml` | 只记录失败请求（>=400）的访问日志 |
| `istio_ServiceEntry.yaml` | connector 访问的外部电商平台 HTTPS 接口 |

入口 Gateway / VirtualService / DestinationRule **不在这里**，由 chart 渲染（避免同一对象被 Helm 与 kubectl 双重管理）。
HTTPS：`--set istio.gateway.tls.credentialName=<证书 Secret>`（Secret 要建在入口网关 Pod 的命名空间），80 端口随后只做跳转。

## 授权策略

ServiceAccount 名 = chart 里各 Deployment 的名字（`services.<svc>.name`）。

| 被调用方 | 谁可以调用 | 路径 |
|----------|-----------|------|
| `couponkill-gateway` | Istio 入口网关 | 全部 |
| `couponkill-user-service` | gateway | 全部公开接口（`/internal/v1/users/*` 目前没有调用方，不对其他服务开放） |
| `couponkill-coupon-service` | gateway | 全部公开接口 |
|  | order | 仅 `/internal/v1/stock/*`、`/internal/v1/coupons/*` |
|  | connector | 仅 `/internal/v1/stock/sync` |
| `couponkill-order-service` | gateway | 全部公开接口 |
|  | coupon | 仅 `/internal/v1/orders/grant` |
| `couponkill-connector-service` | gateway | 全部 |
| 所有 couponkill 工作负载 | （DENY）gateway 的身份访问 `/internal/*` | |
| 所有 couponkill 工作负载 | `monitoring` 命名空间 | 仅 `GET /metrics` |

PostgreSQL / Redis / Kafka 与 `kafka-topic-init` Job 在 chart 里声明了 `sidecar.istio.io/inject: "false"`：它们是纯 TCP 依赖，
留在网格之外，访问控制由 chart 的 NetworkPolicy 负责；业务服务访问它们时 Istio 自动退回明文。

> 一个工作负载上只要存在 ALLOW 策略，没有命中任何规则的请求一律被拒。**新增服务间调用关系时，必须同时修改
> `istio_AuthorizationPolicy.yaml`**，否则会得到 `403 RBAC: access denied`。

## 部署

前提：集群已装 Istio，已用 Helm 部署 chart 且 `istio.enabled=true`。

```bash
# 1. 命名空间打标签并重启已有 Pod 以注入 sidecar（Helm 的 --create-namespace 已建好命名空间）
kubectl label namespace couponkill istio-injection=enabled --overwrite
kubectl -n couponkill rollout restart deploy

# 2. 叠加网格安全 / 可观测配置（istio_config.yaml 只是标签的声明式写法，步骤 1 已做可跳过）
kubectl apply -f k8s-istio/istio_PeerAuthentication.yaml \
              -f k8s-istio/istio_AuthorizationPolicy.yaml \
              -f k8s-istio/istio_Sidecar.yaml \
              -f k8s-istio/istio_Telemetry.yaml \
              -f k8s-istio/istio_ServiceEntry.yaml
```

apply `istio_PeerAuthentication.yaml` 之前，先确认所有业务 Pod 都已带 sidecar（`kubectl -n couponkill get pod`，READY 为 `2/2`），
否则 STRICT 会让无 sidecar 的客户端被拒。

## 验证

```bash
kubectl -n couponkill get authorizationpolicy,peerauthentication,sidecar,telemetry,serviceentry
# 外部只能到 gateway：这几条应该都是 404
curl -i http://<入口地址>/metrics
curl -i http://<入口地址>/readyz
curl -i http://<入口地址>/internal/v1/stock/deduct
# 业务接口正常
curl -i http://<入口地址>/api/v1/coupon/available
# 网格内：gateway 的身份访问 coupon 的内部接口应被拒（403）
istioctl x authz check deploy/couponkill-coupon-service -n couponkill
```

## 从旧版（Java / Nacos 时代）升级

`kubectl apply` 不会清理集群里已有的对象。旧版本遗留的下面这些必须手动删除，否则直连业务服务的绕过路由、
指向已不存在服务账户（`seckill-go`、`couponkill-gateway-service`、`couponkill-infra` 的旧授权）的策略会继续生效：

```bash
kubectl -n couponkill delete virtualservice seckill-go-vs order-service-vs coupon-service-vs user-service-vs \
  nacos-vs sentinel-vs internal-services-vs internal-seckill-go-vs internal-order-service-vs \
  internal-coupon-service-vs internal-user-service-vs --ignore-not-found
kubectl -n couponkill delete authorizationpolicy allow-ingress allow-internal-services seckill-go-policy --ignore-not-found
kubectl -n couponkill delete destinationrule seckill-go-dr order-service-dr coupon-service-dr user-service-dr seckill-go-svc-dr --ignore-not-found
kubectl -n istio-system delete gateway couponkill-gateway --ignore-not-found   # 旧的 istio-system 网关；新的由 chart 渲染在应用命名空间
kubectl -n couponkill delete deploy,svc,sa seckill-go --ignore-not-found
```

## 排障

```bash
istioctl proxy-status
istioctl proxy-config route <pod>.couponkill
istioctl analyze -n couponkill
kubectl -n couponkill logs <pod> -c istio-proxy | grep -E 'RBAC|403'
```

* `503 UC / UF`：目标服务没有 sidecar 却被 DestinationRule 要求 `ISTIO_MUTUAL`——确认命名空间已注入。
* `403 RBAC: access denied`：调用关系不在授权策略里，见上表。
* 外部拿不到 `/metrics`：这是预期行为，Prometheus 在集群内通过 Service 抓取（chart 的 `monitoring.*`）。
