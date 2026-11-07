# coupon 服务（Go）

优惠券目录 + 分桶库存 + 管理员发券。默认端口 `8080`，PostgreSQL schema `cpn`，设计见 `docs/GO-REWRITE-DESIGN.md` §7（cpn）、§8（库存键）、§10（coupon）。

```
cmd/coupon/main.go        只做装配：加载配置 → 日志 → coupon.New → app.Run
internal/coupon/
  server.go               Config、Build（迁移 → 演示种子 → 预热 → 路由）、SeedDemo
  handler/                HTTP 适配：路由、绑定校验、与前端类型一致的 JSON
  service/                业务规则（只依赖自己定义的 Repo / Cache 接口，可用假实现单测）
  repo/                   PostgreSQL 实现；库存不变式在这里、在真实 PG 上证明
  cache/                  Redis 实现：ck:stock:{id} 计数器、ck:cpn:meta:{id} 失效
  migrations/             cpn 的迁移与 seed_demo.sql（只读，由 platform-data 维护）
```

## 配置（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `CK_ENV` | `dev` | `prod` 时密钥缺失/等于开发默认值会启动失败，且**禁止** `CK_SEED_DEMO=true` |
| `CK_HTTP_PORT` | `8080` | 监听端口 |
| `CK_LOG_LEVEL` | `info` | `debug\|info\|warn\|error` |
| `CK_PG_DSN` | 本地 compose | PostgreSQL（含口令，已脱敏输出） |
| `CK_REDIS_ADDR` / `CK_REDIS_PASSWORD` / `CK_REDIS_DB` | `127.0.0.1:6379` / 空 / `0` | Redis |
| `CK_INTERNAL_TOKEN` | 开发默认值 | `/internal/v1/**` 要求的 `X-Internal-Token`（也用于调用 order） |
| `CK_GATEWAY_SIGN_KEY` | 开发默认值 | 校验网关签名的身份头（≥32 字节） |
| `CK_ORDER_URL` | `http://127.0.0.1:8082` | 管理员发券调用的 order 内部地址 |
| `CK_COUPON_STOCK_BUCKETS` | `8`（1..64） | **新建券**时把库存拆成的桶数；已存在的券沿用自己的桶数，事后改它不影响老券 |
| `CK_COUPON_PREHEAT_CONCURRENCY` | `8`（1..64） | 批量预热并发度 |
| `CK_COUPON_INPUT_TZ` | `Asia/Shanghai` | 解释不带时区的时间入参（`yyyy-MM-dd HH:mm:ss`）的 IANA 时区；带时区的 RFC3339 不受影响 |
| `CK_SEED_DEMO` | `false` | `true` 时启动写入演示券 `1001`（秒杀，满100减50）与 `1002`（常驻，满50减10），各 8 桶每桶 200；幂等 |

启动顺序固定：`dbx.Open → dbx.Migrate(cpn) → [seed] → PreheatAll → 挂载路由 → 对外服务`。预热失败（例如 Redis 暂不可用）只记 Error 日志、不阻止启动；`/readyz` 会反映 PostgreSQL 与 Redis 的状态。

## 对外接口（经网关，`/api/v1/coupon`）

响应一律是信封 `{"code":200,"message":"ok","data":...}`；失败带真实 HTTP 状态码（400/401/403/404/409/503）与 `{"code":10002,...,"data":null,"requestId":"..."}`。所有 ID 以字符串输出。

### 公共（可匿名；管理员额外可见停用券）

| 方法 路径 | 说明 |
|---|---|
| `GET /available` | 启用且秒杀时间窗**尚未结束**的券（含「即将开始」，前端据此展示预约；已售罄的仍列出，剩余量为 0），`data` 是数组，最多 1000 条，创建时间倒序 |
| `GET /list?page=1&size=20` | 分页（`size` 最大 100，越界 400）：`data={"items":[...],"total":N,"page":P,"size":S}`；管理员含停用券，其余只含启用券 |
| `GET /{id}` | 单张券；停用券对非管理员返回 404 |

券的 JSON（字段名与前端 `types/api.ts` 的 `Coupon` 一致）：

```json
{"id":"1001","name":"演示秒杀券-满100减50","description":"...","type":2,
 "faceValue":50.00,"minSpend":100.00,"validDays":7,"perUserLimit":1,
 "totalStock":1600,"remainingStock":1600,
 "status":1,"seckillStartAt":"2030-03-01T10:00:00Z","seckillEndAt":"2030-03-01T12:00:00Z",
 "createdAt":"2026-11-07T07:51:59.935038Z","updatedAt":"2026-11-07T07:51:59.935038Z"}
```

`totalStock` 是发行总量，`remainingStock` 是各库存桶之和（**常驻券与秒杀券都输出**；不再有 `seckillTotalStock/seckillRemainingStock/shardIndex/version`）。金额是两位小数的 JSON 数字（不经过浮点数）。时间是 RFC3339 UTC。

### 管理（需 `admin` 角色；非管理员 403，未认证/签名无效 401）

| 方法 路径 | 说明 |
|---|---|
| `POST /create` | 创建券：同一事务写券与各库存桶（总库存均分，余数多分给编号小的桶）；提交后写 `ck:stock`（仅启用的秒杀券）并删 `ck:cpn:meta` |
| `POST /{id}/seckill-window` | 改秒杀时间窗（仅秒杀券，start 必须早于 end），返回更新后的券 |
| `POST /{id}/status` | `status` 为 `0` 停用 / `1` 启用，`data` 为受影响券数（1）。停用会删除 `ck:stock` |
| `DELETE /{id}` | 删除券与库存桶（库存流水保留作审计），`data` 为删除券数；不存在时幂等返回 `0`（沿用 Java 语义） |
| `POST /admin/grant` | 管理员给用户发券，见下文 |
| `POST /preheat-stock` | 把所有启用秒杀券的 `ck:stock` **对齐到 DB 剩余量**（覆盖写），`data={"total","preheated","failed"}`；部分失败 503 |
| `POST /preheat-stock/{id}` | 同上，单张；非秒杀券 400、停用券 409、不存在 404；`data={"couponId","stock"}` |

`/create`、`/{id}/seckill-window`、`/{id}/status` 同时接受 **JSON 体**与**查询串**（现有前端用 axios `params` 传参）。时间既可以是 RFC3339，也可以是 Java 沿用的 `yyyy-MM-dd HH:mm:ss`（按 `CK_COUPON_INPUT_TZ` 解释）。

```bash
# 创建秒杀券（经网关，需要管理员 JWT）
curl -X POST http://localhost:8088/api/v1/coupon/create -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' -d '{
  "name":"秒杀券","type":2,"faceValue":50,"minSpend":100,"validDays":7,"perUserLimit":1,
  "totalStock":1000,
  "seckillStartAt":"2030-03-01 10:00:00","seckillEndAt":"2030-03-01 12:00:00"}'

# 旧前端风格：查询串
curl -X POST "http://localhost:8088/api/v1/coupon/1001/status?status=0" -H "Authorization: Bearer $ADMIN"
```

创建参数：`name`（1..100 字符）、`type`（1|2）、`faceValue`/`minSpend`（非负，最多两位小数，整数部分 ≤ 8 位）、`validDays`（1..3650）、`perUserLimit`（1..1000）、`totalStock`（0..10,000,000）、可选 `id`（不传则雪花生成，已存在 409）、`seckillStartAt/seckillEndAt`（秒杀券必填成对；常驻券不得设置）。

#### 管理员发券 `POST /admin/grant`

```json
{"couponId":"1001","userId":"42","requestId":"optional-idempotency-key"}
→ {"code":200,"data":{"requestId":"grant-377...","orderId":"900...","alreadyGranted":false}}
```

顺序：先用同一个 `requestId` 调**本服务**的 `stock/deduct`（进程内），再调 order 的 `POST /internal/v1/orders/grant`；成功后让 `ck:stock` 同步减一（保持「Redis = DB 剩余」）。失败语义：

- 扣减失败（售罄 409 / 券不存在 404 / 停用 409）：直接返回，没有副作用；
- order **明确拒绝**（4xx，如用户已持有该券 409 `RepeatSeckill`）：幂等 `stock/release` 归还库存后返回原错误；
- 结果**未知**（网络错误/超时/5xx）：订单可能已建成，**不归还**，返回 503 且消息带 `requestId=...`；用同一个 `requestId` 重试即可（deduct 与 grant 都按 requestId 幂等）。

## 内部接口（`X-Internal-Token`，网关对 `/internal/**` 一律 404）

类型与路径见 `internal/contracts/internalapi`（`doc.go` 的「服务端语义要求」是本服务的规格）。

| 方法 路径 | 语义 |
|---|---|
| `POST /internal/v1/stock/deduct` `{requestId,couponId,userId}` | 按 requestId **幂等**扣一件：重放返回第一次的桶并置 `alreadyDeducted=true`；全桶售罄 409 `OutOfStock`；券不存在 404；停用 409；对**已归还**的 requestId 再扣 409 `OutOfStock`；requestId 已用于别的券 400 |
| `POST /internal/v1/stock/release` `{requestId,couponId}` | 幂等归还，只动 DB 桶（Redis 回补是 order 的职责）。从未扣减过则写入「已归还」墓碑（`released` 与 `alreadyReleased` 都为 false），使迟到的 deduct 被拒绝 |
| `POST /internal/v1/stock/sync` `{couponId,available,source,mode?}` | 按 `mode` 调整可用库存（available ≤10,000,000）并刷新 Redis：**缺省/`min` 只降不升**（生效值 = min(当前剩余, available)，目标不低于当前值则什么都不改；外部平台读数可能是恒定或过期值，按它设置会超卖），`set` 才强制设置为绝对值并把发行总量抬到不小于它。锁券行并按桶号顺序锁住全部库存桶后在一个事务里完成；响应的 `available` 是**实际生效值**；提交后 `ck:stock` 对齐 DB（停用券/常驻券则删除键）。 |
| `GET /internal/v1/coupons/{id}` | `CouponInfo`，order 用来校验券并快照，不区分启用状态 |

```bash
curl -X POST http://coupon:8080/internal/v1/stock/deduct -H "X-Internal-Token: $CK_INTERNAL_TOKEN" \
  -H 'Content-Type: application/json' -d '{"requestId":"req-1","couponId":"1001","userId":"42"}'
# {"code":200,"message":"ok","data":{"couponId":"1001","bucket":4,"alreadyDeducted":false}}
```

## 库存不变式（repo 包文档有完整推理）

- 每个 `requestId` 至多扣一次库存：`stock_ledger(request_id PK)` 行先于桶更新「占位」（`INSERT … ON CONFLICT DO NOTHING`），同 requestId 的并发调用在唯一索引上排队，后到者看到已提交的流水后直接返回第一次的桶。
- 崩溃安全：占位、桶更新、回填桶号在**同一个事务**里，任何一步崩溃都整体回滚。
- 售罄 ≠ 冲突：从 `hash(userId) % N` 桶起循环扫描（murmur3 终结函数打散雪花 ID），只有每个桶都没有库存才算售罄；桶被别的事务锁住是等待，不是失败，没有版本号/乐观锁。
- 桶编号 `0..N-1` 连续，N 取券**现有**桶数（`CK_COUPON_STOCK_BUCKETS` 只影响新券）；所有桶 `remaining >= 0` 由 CHECK 兜底。

## Redis

| 键 | 谁写 | 说明 |
|---|---|---|
| `ck:stock:{id}` | **本服务**（创建/状态/时间窗/删除/同步/预热/发券）与 order（Lua） | 只在「启用的秒杀券」上存在，值 = DB 剩余；**不设过期**（它是账本）。键不存在 = 未预热，order 返回 `NotPreheated` |
| `ck:cpn:meta:{id}` | order 写缓存、**本服务删除** | 任何影响券元数据/可用性的变更提交后都会删除，order 未命中回源 `GET /internal/v1/coupons/{id}` |

- DB 先提交、Redis 后刷新，同一张券的刷新在进程内串行。DB 已提交但 Redis 刷新失败时返回 **503**，消息说明「已保存，但库存缓存刷新失败，请调用预热接口重试」；DB 里的数据不回滚。
- **启动预热只写缺失的计数器**（`SETNX`），不覆盖在途账本：重启不会把正在进行的秒杀的预扣计数重置。**管理员预热**与**同步**则无条件对齐到 DB 剩余量。多副本时刷新串行性只在单进程内保证，最终值仍以「最后一次读 DB 的刷新」为准，且 DB 扣减永远是最终闸门，所以最坏情况是多放行几个请求并在 DB 处得到 `OutOfStock`，不会超卖。

## 测试

```bash
export CK_TEST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable'   # 需要真实 PostgreSQL；CI 另设 CK_REQUIRE_PG=true 防止因跳过而变绿
go vet ./cmd/coupon/... ./internal/coupon/... && go test -count=1 ./internal/coupon/...
```

- `service`：假 Repo/Cache 上的规则测试（校验、状态机、预热策略、发券的成功/拒绝/结果未知/客户端断开）。
- `repo`：真实 PostgreSQL 上的不变式——200 并发抢 50 件恰好 50 成功且流水 50 行；同 requestId 并发 20 次恰好生效 1 次；归还后再扣被拒；无扣减先归还写墓碑；重复归还空操作；同步重分配；崩溃注入（桶已更新、未提交）无任何残留；混合负载下 `remaining + 已扣流水 = 初始库存`。
- `cache`：miniredis 上的键语义（SET/SETNX/DEL、`TakeStock` 绝不造出负数计数器）。
- `handler`：真实 PG + miniredis 的端到端 HTTP 测试——匿名/普通用户/管理员矩阵、内部令牌、信封与状态码、JSON 字段名、预热与缓存失效。
- `coupon`（包根）：启动装配——种子幂等、重启保留在途计数器、停用券不预热、Redis 故障不阻止启动。

本机没有 cgo，无法跑 `-race`；CI 里加 `-race`。
