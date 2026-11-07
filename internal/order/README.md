# order 服务（:8082）

秒杀状态机与订单域。设计依据 `docs/GO-REWRITE-DESIGN.md` §8、§9、§10。

| 包 | 职责 |
|---|---|
| `internal/order`（`server.go`、`config.go`） | 环境变量配置；`New(Deps, ...Option)` 装配路由与后台任务 |
| `internal/order/seckill` | 秒杀入口、Lua 脚本、建单消费者、超时清扫器、`Restore`、`ResultHook` |
| `internal/order/orders` | 订单存储原语、普通券下单/取消/查询、管理员发券、HTTP 处理器 |
| `internal/order/reservation`、`notification` | 由 order-aux 提供，经 `WithRoutes` / `WithResultHook` 接入 |
| `cmd/order` | 只做装配：配置 → 日志 → PG(+迁移 `ord`) → Redis → Kafka → coupon 客户端 → `app.Run` |

## 配置（环境变量）

公共项（`CK_ENV`、`CK_HTTP_PORT`（默认 8082）、`CK_PG_DSN`、`CK_REDIS_*`、`CK_KAFKA_BROKERS`、`CK_INTERNAL_TOKEN`、
`CK_GATEWAY_SIGN_KEY`、`CK_COUPON_URL` ……）见 `internal/platform/README.md`。order 私有项：

| 变量 | 默认 | 说明 |
|---|---|---|
| `CK_ORDER_SECKILL_COOLDOWN` | `0s` | 同一用户对同一张券两次秒杀的冷却，0 关闭 |
| `CK_ORDER_ENQUEUE_TIMEOUT` | `500ms` | 发布 Kafka 时同步等待 ack 的上限，超时按失败回滚并返回 503 |
| `CK_ORDER_CONSUME_ENABLED` | `true` | 本实例是否运行建单消费者 |
| `CK_ORDER_CONSUME_CONCURRENCY` | `1` | 同时处理的分区数 |
| `CK_ORDER_CONSUME_MAX_RETRIES` | `5` | 失败重试次数，耗尽进 `seckill_order_create_dlt` |
| `CK_ORDER_SWEEP_ENABLED` | `true` | 本实例是否运行清扫器（多实例靠 `ck:lock:seckill-sweeper` 互斥） |
| `CK_ORDER_SWEEP_INTERVAL` | `30s` | 清扫间隔 |
| `CK_ORDER_SWEEP_STALE` | `120s` | 在途超过多久算超时（必须远大于一次建单的最长耗时） |
| `CK_ORDER_SWEEP_BATCH` | `100` | 每轮最多处理的请求数 |
| `CK_ORDER_MAX_ACTIVE_COUPONS` | `15` | 用户同时持有的券总数上限，0 关闭（Java 版口径） |
| `CK_ORDER_MAX_ACTIVE_SECKILL` | `5` | 用户同时持有的秒杀券数上限，0 关闭 |
| `CK_ORDER_MAX_PAGE_SIZE` | `100` | 列表页大小上限（超出被截断） |
| `CK_ORDER_ADMIN_MAX_OFFSET` | `10000` | 管理端列表最大偏移，超出返回 400 |

## 接口

公开（均经网关签名身份，写接口用 JSON 体，也兼容同名查询参数）：

| 方法与路径 | 说明 |
|---|---|
| `POST /api/v1/order/seckill` `{couponId}` | 受理 **202** `{requestId,status:"QUEUED"}`；拒绝：409（10002 售罄/10010 重复/10006 未开始/10007 已结束/10014 停用/10005 未预热）、429（10001 冷却）、503（50003） |
| `GET /api/v1/order/seckill/result?requestId=` | `{status,orderId?,reason?}`；非属主与不存在都是 404/10011 |
| `POST /api/v1/order/create` `{couponId}` | 常驻券；秒杀券 400/10012 |
| `POST /api/v1/order/cancel` `{orderId}` | `true`=已处于取消态（幂等），`false`=已使用/已过期；别人的订单 404/10011 |
| `GET /api/v1/order/check/{couponId}` | 是否持有未取消订单 |
| `GET /api/v1/order/user/me?page=&size=` | `{items,total,page,size}`，创建时间倒序（兼容 `pageNum/pageSize`） |
| `GET /api/v1/order/user/{userId}` | 管理员 |
| `GET /api/v1/order/admin` | 管理员；`page size startTime endTime userId couponId status orderId` |
| `GET /api/v1/user/coupon/count` | `{count,userId,totalCount,seckillCount,normalCount,expiredCount}` |

内部：`POST /internal/v1/orders/grant`（`X-Internal-Token`，`internalapi.GrantRequest`）：按 `(user_id, request_id)` 幂等，
`source='admin'`，不触碰库存；重放返回 `alreadyGranted=true`；用户已持有该券 409/10010。

订单 JSON 字段：`id userId couponId`（字符串）`couponName couponType faceValue minSpend status source requestId getTime expireTime useTime? cancelTime? createdAt updatedAt`。

## 秒杀状态机

```
POST /seckill
  校验券(meta 缓存→回源 GetCoupon；启用/秒杀类型/时间窗，窗口任一端为 null 即该端不限)
  enter.lua ─ 1 成功 | 0 售罄 | -2 冷却 | -3 重复 | -4 未预热
  发布 seckill_order_create(key=userId，等 ack ≤ 500ms) ── 失败 ─▶ rollback.lua(ENQUEUE_FAILED) ─▶ 503
  202 QUEUED
        │
        ▼   请求哈希 ck:sk:req:{rid}
   QUEUED ──消费者 advance.lua──▶ PROCESSING ──▶ SUCCESS   (订单+通知同一事务)
      │                               │      └──▶ FAILED(OUT_OF_STOCK | REPEAT | LIMIT_EXCEEDED | COUPON_* | SYSTEM_ERROR)
      └──── sweeper：超时无订单 ──▶ release + rollback.lua ──▶ FAILED(TIMEOUT)；有订单 ──▶ SUCCESS
```

消费者每一步都幂等（`Handle`）：advance(PROCESSING) → 已有订单则直接收尾 → 持有上限 → 读券快照 → `stock/deduct`（幂等）
→ 事务插入订单与通知（`ON CONFLICT (user_id, request_id) DO NOTHING`）→ advance(SUCCESS) → 通知 `ResultHook`。

## 不变式（`seckill/simulation_test.go` 与相关测试逐条验证）

1. 每个 requestId 至多一次有效库存扣减、至多一个订单。
2. 守恒：`Redis 计数 + 在途请求数 + 成功订单数 = 初始库存`，计数永不为负。
3. 重复投递、消费者崩溃、coupon 响应丢失、并发的重复处理都不破坏 1、2。
4. transient 错误（`internalapi.IsTransient`、ctx 取消）只重试、**绝不补偿**；重试耗尽进死信，由 sweeper 收尾。
5. 终态不可改：所有状态变更经 Lua CAS。`rollback.lua` 用请求哈希里的 `rolledBack` 字段保证库存**最多回补一次**，
   取消订单与失败终态共用它；回补前库存键必须存在（INCR-if-exists）。
6. 迟到的订单（sweeper/入队失败已判 FAILED 之后才落库）在置 SUCCESS 时被 CAS 拒绝，订单被取消、库存被归还，不会超卖。
7. 取消只做 `status 1→4`，并发取消只有一个赢；归还可重复执行（自愈）；只有 `source='seckill'` 的订单才回补 Redis 计数与占位，
   管理员发券/常驻券订单只归还 DB 桶。
8. 分区表的唯一冲突报告分区索引名：代码从不按约束名匹配。预期内的重复用 `ON CONFLICT` 吸收，其余唯一冲突先按请求号回查
   （同一请求的并发重放 PostgreSQL 可能先撞上部分唯一索引），查不到才是「用户已持有该券」。

## Redis 约定补充

- `ck:sk:req:{rid}` 哈希在 keys 契约的字段之外多一个 `rolledBack`（回补幂等标记）。
- `ck:cpn:meta:{id}` 的内容是 `internalapi.CouponInfo` 的 JSON；coupon 变更时只需 `DEL`，order 回源后自己回填（TTL 5 分钟）。
- 脚本：`seckill/lua/{enter,rollback,advance}.lua`，`go:embed` 内嵌，经 `redisx.Scripts` 注册。

## 与其他模块的接缝（`order.New`）

```go
srv, _ := order.New(deps,
    order.WithResultHook(reservationModule),                 // seckill.ResultHook：异步终态后回写预约、发通知（尽力而为）
    order.WithRoutes(func(r gin.IRouter) { /* r 是根路径上已套好网关身份的分组，用完整路径注册（reservation.RoutePrefix 等） */ }),
)
srv.Seckill().Enter(ctx, userID, couponID)                   // 预约到点代抢复用同一个入口
app.Run(ctx, app.Options{Handler: srv.Handler(), Workers: srv.Workers(), ...})
```

## 测试

`CK_TEST_PG_DSN=postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable go test -count=1 ./internal/order/...`
需要真实 PostgreSQL（`dbxtest`）；Redis 用 miniredis，Kafka 用 `testkit.FakeBroker`，coupon 用进程内假服务 `orders/orderstest.FakeCoupon`
（真实的 `internalapi` 客户端经 HTTP 调用它，带幂等账本与故障注入）。数据竞争检测需要 cgo，CI 里加 `-race`。
