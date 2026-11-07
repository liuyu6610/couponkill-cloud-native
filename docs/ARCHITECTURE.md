# CouponKill 架构

> 后端全部是 Go（五个服务 + 一个共享平台层）；前端是 React；Operator 是独立的 Go 模块。
> 本文描述**已实现**的架构与它的不变式：改动行为前先改本文，并让对应的测试保持为真。

## 1. 关键决策

| # | 决策 | 说明 |
|---|------|------|
| D1 | 全部后端改为 Go；前端保持 React/TS | 前端按新的错误语义与少量接口变化同步修改 |
| D2 | 自由重新设计（不要求与旧 Java 实现的接口、表结构、消息格式兼容） | 旧 Java 实现已移除；公开 API 路径与字段尽量保持，前端同步修改 |
| D3 | 保持 5 个独立服务，端口不变 | gateway:8088 user:8081 coupon:8080 order:8082 connector:8085 |
| D4 | 单个根 Go 模块 `github.com/liuyu6610/couponkill-cloud-native`，Go 1.27.0 | Operator 是独立模块（k8s 依赖很重） |
| D5 | **去掉 Nacos、Sentinel、ShardingSphere** | 配置 = 环境变量（Helm ConfigMap/Secret 注入）；服务发现 = K8s DNS / 环境变量里的 URL；限流熔断在网关内置；分库分表改为 PostgreSQL 原生分区 |
| D6 | 单个 PostgreSQL 16 库 `couponkill`，每个服务一个 schema | `usr` `cpn` `ord` `cnx`；服务只访问自己的 schema，跨服务只走内部 HTTP API |
| D7 | Kafka 用 `franz-go`，HTTP 用 `gin`，PG 用 `pgx/v5`，Redis 用 `go-redis/v9`，JWT 用 `golang-jwt/v5` | 日志 `log/slog`（JSON），指标 `prometheus/client_golang` |
| D8 | 公开 API 路径保持 `/api/v1/...`，字段名 camelCase，**ID 一律以字符串输出** | 错误用**真实 HTTP 状态码** + 统一信封（见 §4） |
| D9 | 不再实现的东西 | mock-token 接口（安全缺口）、支付/推送 SPI（无实现的空壳）、Go 沙箱旁路、`/order/**` 旧路径双挂、Java 内部的 shard/deduct-with-shard-id 等端点 |

## 2. 仓库布局

```
go.mod
cmd/{gateway,user,coupon,order,connector}/main.go      # 只做装配：加载配置 → 构建依赖 → app.Run
cmd/migrate/main.go                                      # 可选：单独跑迁移（服务启动时也会自动迁移自己的 schema）
internal/
  platform/                  # 共享基础设施，不含业务
    apperr/                  # 业务错误码 + 类型（已写好，勿改码值）
    config/                  # 环境变量加载（struct tag）+ 校验；CK_ENV=prod 时拒绝默认密钥
    logx/                    # slog 初始化、ctx 里的 logger / request-id
    httpx/                   # gin 引擎工厂 + 中间件 + 信封 + 健康检查 + HTTP 客户端
    authx/                   # JWT 签发/校验、口令哈希、网关身份签名
    dbx/                     # pgxpool、WithTx、迁移执行器（embed.FS）、错误判定
    redisx/                  # 客户端、Lua 脚本注册、分布式锁
    kafkax/                  # 生产者 / 消费组封装（重试、死信、优雅停止）
    idgen/                   # snowflake
    limiter/                 # 令牌桶、熔断器
    app/                     # 生命周期：Run(...)、优雅关闭、就绪聚合
    testkit/                 # miniredis / 内嵌 PostgreSQL / 假 Kafka / httptest 辅助
  contracts/                 # 跨服务共享的类型：events、internalapi（请求响应 + 客户端）、公共 DTO
  gateway/ user/ coupon/ order/ connector/   # 各服务：server.go（路由）、handler/、service/、repo/、jobs/
  <svc>/migrations/*.sql     # 各服务自己 schema 的迁移（go:embed）
build/Dockerfile             # 多服务共用：--build-arg SERVICE=order
```

分层约定（**接口在使用方定义**，便于用假实现测试）：`handler`（HTTP/消息适配，薄）→ `service`（业务规则，定义自己依赖的 `Repo`/`Cache`/`Client` 接口）→ `repo`（pgx/redis 实现）。业务规则必须能不依赖 PG/Redis/Kafka 直接单测。

## 3. 配置

全部环境变量，前缀 `CK_`。公共：`CK_ENV`(dev|prod)、`CK_HTTP_PORT`、`CK_LOG_LEVEL`、`CK_PG_DSN`、`CK_REDIS_ADDR/PASSWORD/DB`、`CK_KAFKA_BROKERS`、`CK_JWT_SECRET`(≥32 字节)、`CK_JWT_ISSUER`、`CK_JWT_AUDIENCE`、`CK_JWT_TTL`、`CK_INTERNAL_TOKEN`、`CK_GATEWAY_SIGN_KEY`(≥32 字节)、`CK_USER_URL/COUPON_URL/ORDER_URL/CONNECTOR_URL`。`CK_ENV=prod` 时：密钥缺失或等于开发默认值 → 启动失败。开发默认值只用于 `CK_ENV=dev`，并在日志里告警。每个服务自己的开关见各服务章节，命名 `CK_<SVC>_<NAME>`。

## 4. HTTP 约定

- 信封：`{"code":200,"message":"ok","data":...}`；失败 `{"code":10002,"message":"库存不足","data":null}`。分页：`data={"items":[...],"total":N,"page":P,"size":S}`。
- HTTP 状态码随错误码走（见 `apperr`）：校验 400、未登录 401、无权限 403、不存在 404、冲突/售罄/重复 409、限流/冷却 429、系统繁忙/下游不可用 503、其余 500。成功 200；秒杀受理 **202**。
- 每个响应带 `X-Request-Id`（入口生成或沿用），日志与错误体里都能关联。
- 入参校验失败返回 400 + 具体字段信息，不得 panic；所有 handler 经 recover 中间件兜底返回 500 信封。
- 健康：`GET /healthz`（存活，不查依赖）、`GET /readyz`（就绪，检查本服务依赖：PG/Redis/Kafka 按需），`GET /metrics`（Prometheus，仅内网）。

## 5. 鉴权与身份传播

- 登录：`POST /api/v1/user/login`（JSON `{username,password}`）→ 签发 HS256 JWT：`sub`=用户 ID、`roles`、`iss`、`aud`、`jti`、`exp`。服务端会话：`SET ck:sess:{jti} 1 EX ttl`。登出：删该 jti 对应键（**按令牌吊销**，不会因重新登录复活）。口令 bcrypt（cost 10）。
- 角色来自 `usr.users.role`（`user`/`admin`），不再有 admin 用户 ID 白名单环境变量。
- **网关**：剥离客户端带来的 `X-User-Id/X-User-Roles/X-Authenticated/X-Gateway-*`；校验 JWT（算法固定 HS256、必须有 exp、容忍 30s 时钟偏差、校验 iss/aud、查 `ck:sess:{jti}`，Redis 超时 50ms，`CK_GATEWAY_REVOCATION_FAIL_OPEN` 默认 true）；通过后转发 `X-User-Id`、`X-User-Roles`、`X-Gateway-Ts`、`X-Gateway-Sig=HMAC-SHA256(CK_GATEWAY_SIGN_KEY, "{userId}|{roles}|{ts}")`。
- **下游服务**：只信带有效 `X-Gateway-Sig` 且 `|now-ts|≤60s` 的身份头（`httpx.RequireGatewayIdentity` 中间件）。因此绕过网关直连（例如 Istio 误配）也无法伪造身份。
- **服务间内部调用**：路径前缀 `/internal/v1/`，要求 `X-Internal-Token`（常量时间比较）。网关对 `/internal/**` 一律返回 404。
- 管理接口：由服务内中间件检查 `roles` 含 `admin`（网关不重复判断业务权限，只做认证）。

## 6. 网关（gateway）

- 反向代理（`httputil.ReverseProxy`）：路由表（最长前缀优先）：
  - `/api/v1/user/coupon/**` → order（用户券统计由 order 拥有数据）
  - `/api/v1/user/**`、`/api/v1/auth/**` → user
  - `/api/v1/coupon/**` → coupon
  - `/api/v1/order/**` → order（含 `/order/reservations/**`、`/order/notifications/**`）
  - `/api/v1/connector/**` → connector
- 免登录：`POST /api/v1/user/register`、`POST /api/v1/user/login`、`GET /api/v1/connector/health`、`GET /api/v1/connector/bindings/by-coupon/{id}`、`GET /api/v1/connector/price-compare`、`GET /api/v1/coupon/available|list|{id}`（C 端只读，沿用 Java 现状：check 前端实际调用后确定，宁严勿松）、`/healthz` `/readyz` `/metrics`。
- 限流（内置，替代 Sentinel）：每 IP、每用户令牌桶（可配）；`/api/v1/order/seckill` 单独更严；超限 429。每个上游一个熔断器（连续失败 N 次打开，半开探测）；上游超时可配（默认 3s，秒杀 2s）。
- CORS 白名单（`CK_GATEWAY_CORS_ORIGINS`）、路径规范化（拒绝 `..`、`//`、`;`、反斜杠、编码变体 → 400）、请求体大小上限。
- 可观测：访问日志（含 userId、upstream、耗时、状态）、Prometheus 指标（按路由/状态/上游）。

## 7. 数据模型（PostgreSQL，各 schema 的权威描述；SQL 由迁移文件实现）

通用：主键 `BIGINT`（snowflake，应用生成）；时间 `TIMESTAMPTZ`；金额 `NUMERIC(12,2)`；字符串 `TEXT`；状态用 `SMALLINT` + `CHECK`；外键列手工建索引；迁移幂等可重复执行；每个服务启动时在 advisory lock 下运行自己的迁移，`schema_migrations(version, applied_at)` 记录。

- **usr.users**：`id`、`username UNIQUE NOT NULL`、`password_hash`、`phone`、`role`(`user|admin` CHECK)、`status`、`last_active_at`、`created_at`。全局唯一用户名靠单表 UNIQUE（不再分库）。
- **cpn.coupons**：`id`、`name`、`description`、`type`(1 常规/2 秒杀)、折扣/面额字段（沿用 Java 的字段含义，字段名以前端 `types/api.ts` 为准）、`total_stock`、`status`(0 停用/1 启用)、`per_user_limit`、`seckill_start/seckill_end`、`valid_days`、`created_at/updated_at`。
- **cpn.coupon_stock**：`(coupon_id, bucket) PK`、`remaining INT CHECK (remaining >= 0)`。库存拆成 N 个桶（`CK_COUPON_STOCK_BUCKETS`，默认 8）以分散热点行锁。扣减：从 `hash(userId) % N` 桶起循环，`UPDATE ... SET remaining=remaining-1 WHERE coupon_id=$1 AND bucket=$2 AND remaining>0`，全桶无库存才算售罄（售罄 ≠ 乐观锁冲突，不存在版本号）。
- **cpn.stock_ledger**：`request_id PK`、`coupon_id`、`bucket`、`state`(`deducted|released`)、`created_at/updated_at`。**扣减/归还与桶更新在同一事务**，因此对同一 `request_id` 严格幂等、崩溃安全。
- **ord.orders**：按 `user_id` **HASH 分区（16 个）**；`id`、`user_id`、`coupon_id`、`request_id`、`coupon_name`/折扣字段快照（下单时拷贝，列表不再跨服务查询）、`status`(1 已创建/2 已使用/3 已过期/4 已取消 沿用 Java 语义)、`get_time/expire_time/cancel_time`、`source`(`seckill|normal|reservation|admin`)、`created_at/updated_at`。约束：`UNIQUE(user_id, request_id)`；**部分唯一索引 `UNIQUE(user_id, coupon_id) WHERE status <> 4`**（取消后可再抢）；索引 `(user_id, created_at DESC)`、`(coupon_id)`。
- **ord.reservations**（预约帮抢）、**ord.notifications**（站内通知）：字段沿用 Java 的含义，按 `user_id` 建索引；预约扫描用 `(status, trigger_at)` 部分索引。
- **cnx.platform_sku_bindings / cnx.price_maps**：沿用 Java 的含义与唯一约束；同步任务相关列：`last_sync_at`、`last_sync_status`。

种子数据：仅 `CK_SEED_DEMO=true` 时由 user/coupon 服务写入（演示用户 `demo/demo1234`、管理员 `admin/admin1234`、秒杀券与常驻券），生产禁用；种子口令是测试值，不是秘密。

## 8. Redis 键（统一前缀 `ck:`）

| 键 | 类型 | 说明 |
|---|---|---|
| `ck:stock:{couponId}` | int | 秒杀预扣计数器。**由 coupon 服务预热/同步写入**（创建、启动时、管理员预热、连接器同步），order 服务只通过 Lua 读写。键不存在 = 未预热（返回 `NotPreheated`） |
| `ck:sk:user:{couponId}:{userId}` | string | 用户已占位标记（值=requestId），TTL 24h，防重复秒杀；失败/取消时删除 |
| `ck:sk:cool:{couponId}:{userId}` | string | 冷却（可选，`CK_ORDER_SECKILL_COOLDOWN`，默认关） |
| `ck:sk:req:{requestId}` | hash | `{status,userId,couponId,orderId,reason,updatedAt}`，TTL 24h；`status` ∈ QUEUED/PROCESSING/SUCCESS/FAILED；`/seckill/result` 读它并**校验 userId 属主** |
| `ck:sk:inflight` | zset | member=requestId，score=入队毫秒时间戳；sweeper 扫描超时请求 |
| `ck:cpn:meta:{couponId}` | json | 券元数据缓存（状态、类型、时间窗、限购），TTL 5m，coupon 服务变更时主动删除，order 未命中回源内部 API |
| `ck:sess:{jti}` | string | 登录会话，TTL=令牌剩余寿命 |
| `ck:lock:{name}` | string | 分布式锁（`SET NX PX` + 令牌校验释放 + 续租） |
| `ck:rl:*` | — | 网关限流（若用 Redis 滑窗；默认进程内令牌桶，不依赖 Redis） |

Lua 脚本（`internal/order/seckill/lua/*.lua`，`go:embed`）：
- `enter.lua`：KEYS=stock、userMarker、cooldown、reqHash、inflight；原子完成「已占位? → 冷却? → 键存在? → 库存>0? → DECR → SET 占位标记 → HSET req(QUEUED) → ZADD inflight」，返回 `1` 成功、`0` 售罄、`-2` 冷却、`-3` 重复、`-4` 未预热。
- `rollback.lua`：Kafka 发送失败或终态失败时：库存键**存在才** `INCR`、删占位标记、`HSET req FAILED`、`ZREM inflight`，幂等。

## 9. 秒杀状态机（order 服务，核心）

```
POST /api/v1/order/seckill {couponId}
  1 校验券（meta 缓存：启用、秒杀类型、时间窗）→ NotStarted/ActivityEnded/CouponDisabled
  2 enter.lua → 1 才继续；否则映射错误码（0→OutOfStock, -2→CoolingDown, -3→RepeatSeckill, -4→NotPreheated）
  3 发布 Kafka seckill_order_create {requestId,userId,couponId,enqueuedAtMs}，key=userId，同步等待 ack，超时 500ms
     失败 → rollback.lua，返回 503 SystemBusy
  4 返回 202 {requestId, status:"QUEUED"}
前端轮询 GET /api/v1/order/seckill/result?requestId=... → {status, orderId?, reason?}（非属主 404）

消费者（消费组 seckill-order-create-group，手动提交，失败重试带退避，超限进 seckill_order_create_dlt）
  对每条消息，处理必须幂等（任何一步都可能重放）：
  1 读 ck:sk:req:{rid}：已是 SUCCESS/FAILED → 直接提交位点
  2 置 PROCESSING
  3 coupon 内部 POST /internal/v1/stock/deduct {requestId,couponId,userId}（幂等；已扣则返回原桶）
       409 OutOfStock → 终态 FAILED(OUT_OF_STOCK)，rollback.lua
       超时/5xx → 返回错误让消费者重试（同 requestId 重试是安全的）；耗尽重试进 DLT，**不**补偿（交给 sweeper）
  4 单事务：INSERT ord.orders ... ON CONFLICT (user_id, request_id) DO NOTHING + INSERT ord.notifications
       违反 (user_id, coupon_id) 部分唯一索引 → 终态 FAILED(REPEAT)：stock/release + rollback.lua（保留占位标记）
  5 终态 SUCCESS：HSET req，ZREM inflight，提交位点

sweeper（order 服务内定时任务，持有 ck:lock:seckill-sweeper）
  每 30s 取 inflight 中 score < now-120s 的 requestId（每轮 ≤100）：
    按 (user_id, request_id) 查订单 → 存在 ⇒ 终态 SUCCESS
    不存在 ⇒ stock/release（幂等）+ rollback.lua ⇒ 终态 FAILED(TIMEOUT)

取消订单：单事务置 status=4（仅当前为 1）→ 成功后 stock/release（按订单 request_id，幂等）+ 库存键存在才 INCR + 删占位标记
```

不变式（测试必须覆盖）：每个 requestId 至多扣一次库存、至多生成一个订单；Redis 预扣计数 + 在途请求数 + 已成功订单数守恒；重复投递、消费者崩溃、coupon 响应丢失、Kafka 重放、并发取消与支付都不破坏上述不变式。

## 10. 各服务职责与 API

**user（8081）**：`POST /api/v1/user/register`、`POST /login`（JSON 体；口令永不进 URL）、`POST /logout`（吊销当前 jti；网关身份头里没有 jti，所以网关必须原样转发 `Authorization`，user 服务据此验签取 jti 并核对 `sub`）、`GET /profile`、`POST /api/v1/user/batch/generate`（admin，压测造用户，JSON 体 `{prefix,start,count,password?}`，整批共用一个 bcrypt 哈希）。登录对未知用户做等价 bcrypt 比较（防枚举/时序），错误统一「用户名或密码错误」。登录刷新 `last_active_at`（每用户每小时至多一次）。`/internal/v1/users/{id}`（可选，内部取展示名）。

**coupon（8080）**：公共：`GET /api/v1/coupon/available`、`/list`、`/{id}`；管理（admin）：`POST /create`、`POST /{id}/seckill-window`、`POST /{id}/status`、`DELETE /{id}`、`POST /admin/grant`（调 order 内部 `POST /internal/v1/orders/grant`）、`POST /preheat-stock`、`POST /preheat-stock/{id}`；内部：`POST /internal/v1/stock/deduct`、`/stock/release`、`/stock/sync {couponId,available,source,mode?}`（连接器同步平台库存并刷新 Redis；**缺省/`min` 只降不升**：生效值 = min(当前剩余, available)，因为平台读数可能是恒定值或过期值，按它「设置」会把已卖出的库存重置回去造成超卖；只有显式 `set`（管理员 force）才可抬高；响应的 `available` 是实际生效值）、`GET /internal/v1/coupons/{id}`。创建秒杀券/改状态/改时间窗后：事务提交后删除 `ck:cpn:meta:*` 并预热 `ck:stock:*`。启动时对启用中的秒杀券预热（取代 `coupon.cache.preload`）。没有分片/补偿/异步扣减等内部端点：库存扣减与归还只有 `stock/deduct`、`stock/release` 两个幂等入口。

**order（8082）**：`POST /api/v1/order/seckill`、`GET /seckill/result`、`POST /create`（常驻券；秒杀券返回 `10012` 要求走 seckill）、`POST /cancel`、`GET /check/{couponId}`、`GET /user/me`、`GET /user/{userId}`（admin）、`GET /admin`（admin 分页，页大小与最大偏移封顶）、`GET /api/v1/user/coupon/count`；预约：`/api/v1/order/reservations`（POST、DELETE /{id}、GET /mine、GET /{id}）；通知：`/api/v1/order/notifications`（GET /mine、/unread-count、POST /{id}/read、/read-all）；内部：`POST /internal/v1/orders/grant`。预约扫描任务用 `FOR UPDATE SKIP LOCKED`，到点代用户走与手动秒杀同一条流水线（入口函数复用），结果回写预约并产生通知。

**connector（8085）**：13 个端点（绑定、价格映射、同品比价、探测、同步、健康、平台列表）；平台适配器接口 `PlatformAdapter`（jd/pdd/tb/mock，外部调用统一超时 + 重试 + 熔断，参数用环境变量，缺凭证即禁用该平台）；库存同步任务持 `ck:lock:connector-sync`，逐条续租，批次有时间预算，**最久未同步优先**，失败也刷新 `last_sync_at`，结果经 coupon 内部 `stock/sync` 落地。管理员接口要求 admin 角色。

## 11. 测试策略（TDD，纵向切片：一个测试 → 一段实现）

- 单测通过**公开接口**验证行为，不 mock 内部协作者；只在系统边界（PG、Redis、Kafka、外部平台 HTTP）用替身：Redis → `miniredis`（支持 Lua）；Kafka → `testkit.FakeBroker`；外部 HTTP → `httptest`；PG → `testkit.PG(t)`：优先读 `CK_TEST_PG_DSN`，否则启动内嵌 PostgreSQL（`fergusstrange/embedded-postgres`），都不可用则 `t.Skip` 并给出原因（CI 必须配置 DSN，不得因跳过而绿）。
- 必须有的测试：错误码/HTTP 映射表；JWT 与网关签名（篡改、过期、重放时间窗）；每个 handler 的成功与失败路径；§9 不变式（并发 + 重放 + 崩溃注入）；SQL 约束（部分唯一索引、分区、幂等迁移跑两次）；网关路由/鉴权/限流/熔断；connector 同步预算与轮转。
- 命令：`go vet ./cmd/... ./internal/... && go test -count=1 ./cmd/... ./internal/...`。数据竞争检测需要 cgo（本机无 gcc）：CI 里加 `-race`。

## 12. 交付与部署

- `build/Dockerfile`：多阶段、`golang:1.27-alpine` 构建、`distroless/static:nonroot` 运行，`ARG SERVICE`；镜像 `couponkill-<svc>`；探针 `/healthz` `/readyz`。
- Helm（`charts/couponkill`）：5 个 Deployment + ConfigMap/Secret 注入 `CK_*`；删除 nacos、sentinel、db-init（迁移由服务自己执行）；保留 PG/Redis/Kafka、KEDA（消费组 `seckill-order-create-group`、主题 `seckill_order_create`）、Istio（仅暴露 gateway）、监控；DLT 主题预建。
- CI：Go（vet + test + 在 `services: postgres, redis` 下跑集成测试）、前端（lint/test/build）、Helm 渲染断言、Operator。
- 本地：`docker-compose.migration.yml` 保留 PG/Redis/Kafka，移除 Nacos；`scripts/` 里的冒烟/压测脚本改为对新 API；新增 Go 编写的 E2E（`test/e2e`，`CK_E2E_BASE_URL` 指向运行中的栈）：注册→登录→查券→秒杀→轮询→取消→再抢。

## 13. 代码地图与完成定义

| 路径 | 内容 |
|---|---|
| `cmd/<svc>/main.go` | 只做装配：配置 → 日志 → 依赖 → `app.Run`；`cmd/devinfra` 是无 Docker 时的 Redis/Kafka 开发替身 |
| `internal/platform/**` | 共享基础设施，API 速查见 `internal/platform/README.md` |
| `internal/contracts/**` | 跨服务契约：`keys`（Redis 键）、`events`（Kafka 载荷）、`internalapi`（内部端点与客户端） |
| `internal/<svc>/**` | 各服务：`handler`（HTTP 适配）→ `service`（业务规则，自带依赖接口）→ `repo`（pgx / redis）；每个服务目录有 README |
| `internal/<svc>/migrations/*.sql` | 各服务自己 schema 的迁移，服务启动时在 advisory lock 下执行 |
| `test/e2e` | 对真实五个服务进程的端到端测试（`CK_E2E_BASE_URL`） |
| `build/Dockerfile`、`charts/couponkill`、`docker-compose.yml`、`k8s-istio` | 交付与部署 |

完成定义：`go vet` 与 `go test` 全绿、每条行为有测试、公开标识符有文档注释、无 TODO/占位。
