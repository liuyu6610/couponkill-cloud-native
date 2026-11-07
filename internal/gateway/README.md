# internal/gateway —— API 网关（设计 §5、§6）

唯一对外暴露的入口（默认 `:8088`）。它只做四件事：**规范化路径、校验身份、保护上游（限流/熔断/超时）、反向代理**。
业务权限（例如 admin）不在这里判断，由下游服务用 `httpx.RequireRole` 判断。

## 请求处理顺序

```
Handler 入口：剥离 X-User-Id / X-User-Roles / X-Authenticated / X-Gateway-*（任何路径都先剥离，含探针）
  /healthz /readyz /metrics        httpx 自带，不经过下面的中间件（不鉴权、不限流）
  其余请求依次经过：
  1 安全头            nosniff / DENY / no-referrer / CSP / Cache-Control: no-store
  2 路径守卫          歧义路径 → 400（见下）
  3 CORS              白名单源；预检（OPTIONS + Access-Control-Request-Method）在此以 204 结束
  4 每 IP 令牌桶      超限 429 + Retry-After（在鉴权之前，挡住无效令牌的洪水）
  5 路由解析          /internal/** 与未知路径 → 404 信封（不要求登录）；否则选定上游、判定是否免登录
  6 请求体上限        声明长度超限 → 413；分块上传在转发时被截断并映射为 413
  7 鉴权              免登录路径直接放行（且不带身份）；其余校验 JWT + 会话，写入已签名身份头
  8 每用户令牌桶      超限 429
  9 秒杀入口令牌桶    仅 POST /api/v1/order/seckill，按用户，更严
 10 熔断器 + 反向代理  每个上游一个熔断器；等待响应头超时（默认 3s，秒杀 2s）
```

## 路由表（最长前缀优先，前缀按路径段边界匹配）

| 前缀 | 上游 |
|---|---|
| `/api/v1/user/coupon/**` | order（用户券统计的数据在 order） |
| `/api/v1/user/**`、`/api/v1/auth/**` | user |
| `/api/v1/coupon/**` | coupon |
| `/api/v1/order/**` | order（含 reservations、notifications） |
| `/api/v1/connector/**` | connector |
| `/internal/**`、其他任何路径 | 404 信封 |

路径原样转发（上游自己注册 `/api/v1/...`），查询串与请求体（流式）原样保留，`Authorization` 也原样转发
（user 服务登出时要自己解析 jti）。上游收到的额外头：`X-Request-Id`、`X-Forwarded-For`（解析后的客户端 IP）、
`X-Forwarded-Proto`、`X-Forwarded-Host`，以及已认证请求的 `X-User-Id` / `X-User-Roles` / `X-Gateway-Ts` / `X-Gateway-Sig`。

## 免登录白名单（依据前端实际调用裁剪，其余一律需要登录）

- `POST /api/v1/user/register`、`POST /api/v1/user/login`
- `GET /api/v1/coupon/available`、`GET /api/v1/coupon/list`、`GET /api/v1/coupon/{数字id}`
  （首页、券列表、券详情、秒杀页在 `App.tsx` 里没有 `ProtectedRoute`）
- `GET /api/v1/connector/health`、`GET /api/v1/connector/bindings/by-coupon/{数字id}`、`GET /api/v1/connector/price-compare`
  （券详情页只读展示）
- `/healthz`、`/readyz`、`/metrics`

免登录请求不校验令牌、不携带身份、也不消耗用户令牌桶。`/api/v1/auth/token/mock` 已不存在（设计 D9）。

## 鉴权细节

- 令牌只从 `Authorization: Bearer ...` 读取，不接受查询参数。
- `authx.Tokens.Verify`：HS256 固定、必须有 `exp`、30s 时钟宽限、校验 `iss`/`aud`。过期 → 401 `20001`，其余不合法 → 401 `20002`。
- 会话检查：`ck:sess:{jti}`（`redisx.SessionStore.Active`），带 `CK_GATEWAY_REDIS_TIMEOUT`。会话不存在（登出/吊销/过期）→ 401。
  Redis 出错或超时：`CK_GATEWAY_REVOCATION_FAIL_OPEN=true` 放行（计入 `ck_gateway_session_check_fail_open_total` 并告警），`false` 则 503。
- `CK_GATEWAY_REVOCATION_ENABLED=false`：完全不访问 Redis，`/readyz` 也不检查 Redis。
- 身份头 `HMAC-SHA256(CK_GATEWAY_SIGN_KEY, "{userId}|{roles}|{ts}")`，下游用 `httpx.RequireGatewayIdentity` 验签并限制 ±60s。

## 路径守卫（对应 Java `InternalApiBlockFilter` 的意图）

转义路径里出现 `%2e` `%2f` `%5c` `%3b` `%25`（不区分大小写，含双重编码），或解码路径里出现 `//`、`\`、`;`、`.`/`..` 段、控制字符
（含 NUL、DEL）→ 400 `10004`「请求路径不合法」。网关按原样转发路径，若不拒绝，下游再归一化一次就会让白名单与实际命中的接口不一致。

## 错误映射（都是信封，不泄露上游细节）

| 场景 | HTTP | 业务码 |
|---|---|---|
| 歧义路径 | 400 | 10004 |
| 未知路径 / `/internal/**` | 404 | 10004 |
| 请求体过大 | 413 | 10004 |
| 缺令牌 / 令牌过期 / 会话失效 | 401 | 20001 |
| 令牌不合法 | 401 | 20002 |
| 限流（IP / 用户 / 秒杀） | 429 + `Retry-After` | 10003 |
| 熔断打开 | 503 | 50003（coupon 上游为 10013） |
| 会话存储不可用且 fail-closed | 503 | 50003 |
| 上游连接失败 | 502 | 50003（coupon 上游为 10013） |
| 上游等待响应头超时 | 504 | 50003（coupon 上游为 10013） |
| 客户端中途断开 | 499（无响应体） | — |

上游自己返回的 4xx/5xx 原样透传。熔断器只统计 5xx 与传输错误/超时；4xx、客户端断开、请求体超限都不计失败。
超时只约束「等到响应头」的时间，不限制响应体的传输（大响应与流式响应不会被截断）。

## 环境变量

公共（`internal/platform/config`）：`CK_ENV`、`CK_HTTP_PORT`（默认 8088）、`CK_LOG_LEVEL`、`CK_JWT_SECRET/ISSUER/AUDIENCE/TTL`、
`CK_GATEWAY_SIGN_KEY`、`CK_USER_URL/COUPON_URL/ORDER_URL/CONNECTOR_URL`、`CK_REDIS_ADDR/PASSWORD/DB`。
`CK_ENV=prod` 时 JWT 密钥与签名密钥必须显式设置。网关私有项：

| 变量 | 默认 | 说明 |
|---|---|---|
| `CK_GATEWAY_CORS_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | 允许跨域的源，逗号分隔，不允许 `*` 与路径；凭据（Cookie）不开 |
| `CK_GATEWAY_IP_RPS` / `_IP_BURST` | 50 / 100 | 每客户端 IP 令牌桶 |
| `CK_GATEWAY_USER_RPS` / `_USER_BURST` | 20 / 40 | 每登录用户令牌桶 |
| `CK_GATEWAY_SECKILL_RPS` / `_SECKILL_BURST` | 2 / 3 | `POST /api/v1/order/seckill` 每用户令牌桶 |
| `CK_GATEWAY_UPSTREAM_TIMEOUT` | 3s | 等待上游响应头的上限 |
| `CK_GATEWAY_SECKILL_TIMEOUT` | 2s | 秒杀入口的上限 |
| `CK_GATEWAY_CONNECTOR_TIMEOUT` | 10s | connector 上游的上限（同步/探测会调外部平台） |
| `CK_GATEWAY_BREAKER_FAILURES` | 5 | 连续失败多少次打开熔断器 |
| `CK_GATEWAY_BREAKER_OPEN` | 30s | 打开多久后半开探测 |
| `CK_GATEWAY_MAX_BODY_BYTES` | 1048576 | 请求体上限 |
| `CK_GATEWAY_TRUSTED_PROXIES` | 空（不信任任何代理） | 受信代理 IP/CIDR；放在 Ingress 后面时填入其网段，否则所有用户共用 Ingress 的 IP 桶 |
| `CK_GATEWAY_REVOCATION_ENABLED` | true | 是否检查会话（需要 Redis） |
| `CK_GATEWAY_REVOCATION_FAIL_OPEN` | true | Redis 出错/超时时放行（false 则 503） |
| `CK_GATEWAY_REDIS_TIMEOUT` | 50ms | 单次会话查询超时 |

## 指标与日志

`/metrics` 除 httpx 的通用指标外：`ck_gateway_proxied_total{upstream,status}`、`ck_gateway_proxy_duration_seconds{upstream}`、
`ck_gateway_rejected_total{reason}`、`ck_gateway_session_check_fail_open_total`、`ck_gateway_breaker_state{upstream}`（0 closed / 1 half-open / 2 open）。
每个转发请求另有一条 `gateway proxied` 日志（`upstream`、`status`、`upstream_ms`、`user_id`），熔断状态变化记 Warn。
`/metrics` 在网关端口上无鉴权，生产应由 Ingress/NetworkPolicy 限制为内网访问。

## 本地运行

```bash
# 依赖：Redis（会话检查）与四个上游；只验证网关本身时可关闭会话检查
CK_GATEWAY_REVOCATION_ENABLED=false go run ./cmd/gateway
curl -i localhost:8088/healthz
curl -i localhost:8088/api/v1/order/user/me        # 401 信封
curl -i localhost:8088/internal/v1/stock/deduct    # 404 信封

# 完整本地栈（上游用默认的 127.0.0.1:8080/8081/8082/8085，Redis 用 docker-compose.migration.yml 的 6379）
CK_REDIS_ADDR=127.0.0.1:6379 go run ./cmd/gateway
```

测试：`go test -count=1 ./internal/gateway/...`（假上游用 `httptest`，Redis 用 miniredis，限流与熔断用假时钟，无需任何外部服务）。
