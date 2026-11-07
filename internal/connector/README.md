# connector —— 电商 Connector 服务（:8085）

平台 SKU ↔ 本地券绑定、同品比价、平台探测，以及把平台库存定时同步给 coupon 服务。
Connector **不在秒杀热路径上**：它只读第三方平台，再通过 coupon 的内部 API `POST /internal/v1/stock/sync`
设置券的可用库存。规格见 `docs/GO-REWRITE-DESIGN.md` §7、§10；Java 行为参考 `couponkill-connector-service`。

## 目录

```
cmd/connector/main.go        只做装配：LoadConfig → logx → connector.Build → app.Run
internal/connector/
  config.go                  环境变量配置（CK_CONNECTOR_*）与校验
  wire.go                    Build：PG 迁移(cnx) → Redis → 适配器/注册表 → service → 引擎 + 定时任务
  server.go                  13 个端点的路由与鉴权矩阵（薄适配层）
  domain/                    领域类型、JSON 形状、本服务专有错误
  platforms/                 PlatformAdapter、Caller(超时/重试/熔断)、Registry(健康把关)、jd/pdd/tb/mock
  repo/                      pgx 仓储（cnx.platform_sku_bindings / cnx.price_maps）
  service/                   绑定、价格映射与比价、探测、库存同步（SyncService / SyncJob / RedisLocker）
  migrations/                cnx schema 迁移（platform-data 负责，本包只读）
```

## 配置（环境变量）

公共项（`CK_ENV`、`CK_HTTP_PORT`(默认 8085)、`CK_LOG_LEVEL`、`CK_PG_DSN`、`CK_REDIS_*`、`CK_INTERNAL_TOKEN`、
`CK_GATEWAY_SIGN_KEY`、`CK_COUPON_URL`、`CK_WORKER_ID`）见 `internal/platform/README.md`。本服务私有：

- `CK_CONNECTOR_SYNC_ENABLED`（true）：是否启动定时同步任务；手工同步接口不受影响。
- `CK_CONNECTOR_SYNC_INTERVAL`（1m）：两轮之间的间隔。第一轮在一个间隔之后开始。
- `CK_CONNECTOR_SYNC_BUDGET`（30s）：单轮时间预算，必须小于锁 TTL。
- `CK_CONNECTOR_SYNC_LOCK_TTL`（60s，下限 10s）：同步锁租约，每条绑定开始前与写库存前都会续满。
- `CK_CONNECTOR_MOCK_STOCK`（500）：MOCK 平台 SKU 不带 `:数量` 后缀时的库存。
- `CK_CONNECTOR_HEALTH_TTL`（30s）：平台健康状态缓存（惰性复查）。
- `CK_CONNECTOR_COMPARE_CACHE_TTL`（30s）：同品比价结果缓存（匿名接口会触发第三方探测，缓存防放大）。
- `CK_CONNECTOR_CALL_RETRIES`（2）、`CK_CONNECTOR_BREAKER_FAILURES`（5）、`CK_CONNECTOR_BREAKER_OPEN_FOR`（30s）：
  对平台幂等读的重试次数；每个平台独立熔断器的打开阈值与半开等待。
- 京东：`CK_CONNECTOR_JD_APP_KEY` / `_APP_SECRET` / `_ACCESS_TOKEN`，可选 `_SERVER_URL`、`_DEFAULT_AREA`。
- 淘宝：`CK_CONNECTOR_TB_APP_KEY` / `_APP_SECRET` / `_SESSION`，可选 `_SERVER_URL`。
- 拼多多：`CK_CONNECTOR_PDD_APP_KEY`（client_id）/ `_APP_SECRET` / `_ACCESS_TOKEN`，可选 `_SERVER_URL`。

凭证都是 `config.Secret`（日志/配置快照只显示 `[REDACTED]`），**缺任何一项凭证，该平台就是 DISABLED**：
不发任何外部请求，启动不失败，`CK_ENV=prod` 也不会因此拒绝启动。MOCK 永远 UP。

## 端点（前缀 `/api/v1/connector`，信封与错误码见设计 §4）

- `GET  /health` —— 各平台 `{platform,status,message}`；status 为 UP / DOWN / DISABLED。**免身份**。
- `GET  /bindings/by-coupon/{couponId}` —— C 端只读；没有绑定时 `data: null`。
- `GET  /price-compare?couponId=` —— C 端只读；主绑定探测 + 启用的手工映射，同一 (平台,SKU) 只出现一次。
- `POST /bindings` —— admin；`{platform,externalSkuId,couponId,syncEnabled?}`，couponId 接受数字或字符串，
  platform 大小写不敏感。同一 (platform,sku) 已存在则更新；一张券只能绑一个 SKU（冲突 409）。
- `GET  /bindings` —— admin；id 降序。
- `POST /price-maps`、`GET /price-maps?couponId=`、`DELETE /price-maps/{id}` —— admin；唯一 (coupon, platform)；
  `manualPrice` 非负、最多两位小数；`currency` 缺省 CNY；删除幂等。
- `POST /sync/{id}?force=`、`POST /sync?force=` —— admin；手工同步，与定时任务**共用同一把锁**；锁被占用 409。
  单个同步返回绑定（平台/推送失败时返回带 `lastSyncStatus=FAIL` 的绑定而不是错误）；全部同步返回
  `{syncedOk,failed,skipped,force}`。
- `GET  /platforms` —— admin；健康状态，JD 条目附带 `jdEnabled/jdCredentialsConfigured/jdServerUrl/jdDefaultArea/jdAppKeyMasked`。
- `GET  /probe/{platform}/product|stock?skuId=` —— admin；探测，不写任何状态。

鉴权矩阵：admin 路由必须有网关签名的身份且角色含 `admin`（匿名 401、普通用户 403、伪造的未签名身份头 401）。
两个 C 端只读接口在网关白名单里匿名可访问且网关不会给它们带身份头，所以允许**完全没有身份头**的请求；
一旦带了任何身份头，签名就必须有效。

JSON 约定：ID 为字符串；时间 `createdAt/updatedAt/lastSyncAt/fetchedAt` 为 RFC3339；金额是两位小数的 JSON 数字
（平台价格统一四舍五入到两位）。

错误映射：未知平台/参数错误 400；平台未启用或熔断中 503；平台调用失败/超时 503；平台业务拒绝或响应无法解析 502；
平台上没有该 SKU 404；绑定不存在 404；同步锁被占用或写前丢锁 409（业务码沿用 `SystemBusy` 50003）。

## 适配器：如何新增一个平台

1. 在 `domain` 增加平台常量并加入 `AllPlatforms`（同时要让 `cnx` 表的 `platform` 约束 `^[A-Z][A-Z0-9_]*$` 接受它）。
2. 在 `platforms` 实现 `PlatformAdapter`：`Platform()`、`Product()`、`Stock()`、`Health()`。
   - 统一用 `Caller.Do(ctx, build)` 发请求：`build` 每次尝试都会被重新调用（重新签名/刷新时间戳），
     返回的错误已分类（`*UpstreamError` 传输/5xx/429 才重试并计入熔断；`ErrCircuitOpen` 快速失败）。
   - 平台 200 响应里的业务错误返回 `*RejectedError`（不重试、不计入熔断）；SKU 不存在返回 `ErrNotFound`；
     缺凭证返回 `ErrDisabled` 且**不发请求**。
   - `Health` 必须便宜（不得外呼）：用 `credentialedHealth`（缺凭证 DISABLED / 熔断打开 DOWN / 否则 UP）。
   - 价格用 `price()` 归一，库存取不到精确数量时 `StockQty=nil` 并如实给 `StockAvailable`。
3. 在 `wire.go` 的 `Build` 里加入注册表，并在 `Config` 里加 `CK_CONNECTOR_<平台>_*` 凭证（`config.Secret`）。
4. 测试用 `httptest` 假平台：校验签名算法（用独立实现或黄金值）、解析夹具、错误映射、熔断生命周期。

TB/PDD 的 Java 版只有“二期预留”的空壳，没有可移植的签名/解析代码：这里的接口名与字段（TB `taobao.item.seller.get`，
PDD `pdd.goods.information.get`，价格单位分）依据两家公开文档手写，**没有对线上账号验证过**，上线前需用真实凭证
走一遍 `/probe`。

## 库存同步算法

```
SyncJob.Run：每 CK_CONNECTOR_SYNC_INTERVAL 触发一轮 SyncService.RunScheduled
RunScheduled：
  ck:lock:connector-sync 拿不到 → 静默跳过（别的实例/手工同步在跑）；Redis 不可用 → 报错、什么都不做
  按“最久未同步优先”（last_sync_at NULLS FIRST, id；由部分索引提供顺序）取启用的绑定
  对每条绑定：
    ① 停止条件：ctx 取消 / 超过预算（仅在每条开始前检查，在途的一条跑完）/ 续租失败（Guard）→ 剩余全部顺延
    ② 读平台库存（单条上限 = 锁 TTL/2）；失败 → 记 FAIL
    ③ 解析目标：有精确数量按平台（含 0）；无数量但无货 → 0；有货却不精确 → 记 SKIP（绝不虚增）
    ④ 写 coupon 前再续租一次；确认失败 → 本条与剩余绑定不写、不记录（last_sync_at 不变，下一轮排最前）
    ⑤ coupon.SyncStock(目标值, mode=min（force 时 set）, source=connector)，超时 5s，不自动重试；失败 → 记 FAIL，下一轮重推
    ⑥ 记录 SUCCESS/FAIL/SKIP，一律刷新 last_sync_at；失败/跳过不抹掉上次成功的 last_stock
```

不变式：任一时刻只有一个持锁者在推送；丢锁后一律不写；成功、失败、跳过都刷新 `last_sync_at`，
所以一条慢而持续失败的绑定不会每轮都排第一把预算吃光（饿死其它绑定）；被预算/丢锁顺延的绑定下一轮自然排最前；
SyncStock 带目标值而不是增量，失败只留给下一轮，重放不会叠加；ctx 取消（服务关闭）不记 FAIL，锁照常释放。

“自动续租”的实现方式：循环是顺序的，每一步的耗时都有上限（平台调用 ≤ TTL/2，推送 ≤ 5s），
所以在每条开始前与写库存前各续满一次（`Lease.Guard`，Lua 比较令牌再 PEXPIRE），效果等同心跳，
而且丢锁可以在最关键的写入点之前被确定性地发现，不依赖后台 goroutine 的调度时机。

### `force` 参数与推送模式

平台给出的库存读数可能是恒定值（mock 恒为 500）或过期值，按它「设置」会把已经卖出的库存重置回去造成超卖。
所以推送给 coupon 的 `stock/sync` 带 `mode`：

- 定时任务、不带 `force` 的手工同步：`mode=min`，只降不升（生效库存 = min(coupon 当前剩余, 平台读数)）；
- `?force=true` 的手工同步（admin）：`mode=set`，设置为平台读数，可以抬高库存。

`SyncBatchResult.force` 回显本次是否强制。
