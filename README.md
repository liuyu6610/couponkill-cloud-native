# CouponKill Cloud-Native – 快速集成包

本补充包包含：
- **user-service**：`/api/v1/auth/token/mock` 生成压测用 JWT
- **couponkill-common**：`JwtUtil` 工具类（HS256）、统一响应/错误码 DTO（若你已引入则复用）
- **OpenAPI**：`api-spec/openapi.yaml` 覆盖秒杀与 mock token
- **Swagger/Knife4j**：提供 Springdoc 配置，访问 `swagger-ui/index.html` 与 `doc.html`
- **Helm Chart**：含 HPA 与 Istio VirtualService 的部署模板与 `values.yaml`
- **架构图**：`architecture.png`（本仓根或 docs 目录引用）

![Architecture](architecture.png)

## 1. user-service（JWT 压测发放）
- 路径：`couponkill-user-service`
- 主要接口：`POST /api/v1/auth/token/mock`
- 请求体：
```json
{"userId":"user1","roles":["user"]}
```
- 响应：`code=0` 且 `data.token` 即 Bearer Token
- 配置（`application.yml`）：
  - `auth.jwt.secret`（默认从 `JWT_SECRET` 环境变量注入，**生产务必更换**）
  - `auth.jwt.issuer/audience/mock-ttl-seconds`

### POM 依赖
见 `couponkill-user-service/pom.xml`，已加入 `jjwt` 与 `spring-boot-starter-web`。

## 2. 在各服务开启 Swagger / Knife4j
在各服务 `pom.xml` 增加（版本可根据你公司镜像源调整）：

```xml
<dependency>
  <groupId>org.springdoc</groupId>
  <artifactId>springdoc-openapi-starter-webmvc-ui</artifactId>
  <version>2.5.0</version>
</dependency>
<dependency>
  <groupId>com.github.xiaoymin</groupId>
  <artifactId>knife4j-openapi3-jakarta-spring-boot-starter</artifactId>
  <version>4.5.0</version>
</dependency>
```

在 `coupon-service`/`order-service`/`user-service` 已添加 `OpenApiConfig`，启动后访问：
- Swagger UI: `http://<host>:<port>/swagger-ui/index.html`
- Knife4j: `http://<host>:<port>/doc.html`

## 3. OpenAPI 规范
统一的 `api-spec/openapi.yaml` 已包含：
- `POST /seckill/{couponId}/enter`
- `GET /seckill/results/{requestId}`
- `POST /auth/token/mock`
可用于网关聚合文档或导入到 Postman/Apifox。

## 4. Helm Chart 一键部署
目录：`charts/couponkill`。核心文件：
- `values.yaml`：镜像仓库/标签、各服务端口与 **HPA** 设置、**Istio Gateway/VirtualService** Host
- `templates/deploy-*.yaml`：Deployment/Service/HPA
- `templates/istio.yaml`：Gateway + VirtualService（路径路由到 user/coupon/order）

### 部署示例
```bash
helm upgrade --install couponkill charts/couponkill       --namespace couponkill --create-namespace       --set image.registry=registry.your.com/couponkill       --set image.tag=$(git rev-parse --short HEAD)       --set istio.gateway.host=couponkill.your.com
```

> 提醒：确保 `istio-system` 中有 `ingressgateway`。如需 TLS，可在 `templates/istio.yaml` 增加 `tls` 配置。

## 5. JMeter 直接拉 Token 造数
1. 先启动 `user-service`；
2. JMeter PreProcessor 里 `POST /api/v1/auth/token/mock` 获取 `data.token`，存入变量 `${TOKEN}`；
3. 测秒杀：`Authorization: Bearer ${TOKEN}`，`X-User-Id` 用与 token 一致的 `userId`。

## 6. Operator 自动扩容（高峰期）
- 使用你此前的 `SeckillScalePolicy` CR（见 `operator/sample/`）。
- 该 CR 会为 **Go 秒杀边车** 与 **coupon/order** 生成/更新 HPA（CPU 60%）。
- 如需 Kafka 积压触发弹性，建议引入 **KEDA**（可在后续版本补充 `ScaledObject` 模板）。

## 7. 目录对齐
- `couponkill-user-service/`（新增）
- `couponkill-common/`：`com.couponkill.common.jwt.JwtUtil`
- `api-spec/openapi.yaml`
- `charts/couponkill/`
- `docs/pom_snippets_springdoc.md`
- `architecture.png`

## 8. 安全与生产注意事项
- **JWT Secret** 放 K8s Secret：`kubectl create secret generic jwt-secret --from-literal=secret=<256bit>`
- Kafka/Redis/MySQL 连接地址改为你的内网服务名
- 压测前置：`SET stock:{couponId} <大库存>`
- 灰度/金丝雀：在 Istio `VirtualService` 中增加 subset 路由 + `DestinationRule`；或使用 Argo Rollouts

---

Happy hacking！
