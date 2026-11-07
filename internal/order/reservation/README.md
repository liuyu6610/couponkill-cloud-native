# internal/order/reservation —— 预约帮抢

用户在秒杀开售前预约；到点由调度器代用户走与手动秒杀**同一条入口**（`Seckiller.Enter`），
结果回写预约并产生站内通知（`internal/order/notification`）。数据在 `ord.reservations`
（迁移见 `internal/order/migrations/0001_init.sql`）。

## 状态机

```
                 到点认领(SKIP LOCKED)      入口受理               建单成功
   PENDING ───────────────────────▶ FIRING ───────────▶ QUEUED ───────────▶ SUCCESS
     │  ▲                              │  │                 │
     │  └─── 瞬态失败 / 崩溃回收 ───────┘  │                 └─ 建单失败 ─────▶ FAILED
     │      (retry_count+1, 退避)          │
     │                                     ├─ 终态业务错误 / 重试耗尽 ─────────▶ FAILED
     └─ 用户取消 ─▶ CANCELLED              ├─ 活动已结束 ─────────────────────▶ EXPIRED
                                           └─ 结果事件先于 QUEUED 回写 ────────▶ SUCCESS / FAILED
```

- 终态：SUCCESS / FAILED / CANCELLED / EXPIRED，不可再变。
- 进行中：PENDING / FIRING / QUEUED；同一用户同一张券至多一个（部分唯一索引 `uk_reservations_active_user_coupon`）。
- 迁移表是 `status.go` 的 `transitions`，所有写状态的 SQL 都由它推导 `WHERE status = ANY(合法来源)`，
  `status_test.go` 对 7×7 种组合在真实 PostgreSQL 上逐一验证。
- 与迁移文件注释的差异：`PENDING → EXPIRED` 不是直接迁移，而是 `PENDING → FIRING → EXPIRED`
  （触发时入口返回“活动已结束”才判定过期，调度器不需要为此回源查券）。

## 到点调度（`Scheduler`）

| 步骤 | 做什么 |
|---|---|
| 回收 | `FIRING` 且 `fired_at` 超过 `StaleFiringTimeout`（认领方崩溃）→ 有重试额度则退回 `PENDING` 并退避，否则 `FAILED` + 通知 |
| 认领 | 单条 `WITH due AS (SELECT … FOR UPDATE SKIP LOCKED) UPDATE … SET status='FIRING'`，多副本并发各拿各的行 |
| 触发 | 每条先 `touch`（刷新 `fired_at` 并确认仍是自己这一轮），再带 `EnterTimeout` 调 `Seckiller.Enter`，最多 `Concurrency` 个并发 |
| 写回 | 所有写入带 fence（认领时的 `retry_count`）：被回收并重新认领后，迟到的写回落空 |
| 回查（可选） | 配了 `ResultReader`：对停在 `QUEUED` 超过 `QueuedSyncAfter` 的预约主动查结果，兜底结果事件丢失 |

入口返回值的分类（`classify`）：

| 入口返回 | 预约结果 |
|---|---|
| 成功 `requestID` | `QUEUED`，记录 `request_id` |
| `apperr.RepeatSeckill` | `FAILED`(10010)：用户已占位。**崩溃恢复时入口其实已受理过**也走这里，文案引导用户去「我的券」确认，不会重复扣库存/下单 |
| `apperr.ActivityEnded` | `EXPIRED` + 过期通知 |
| 其他 4xx 业务错误（库存不足、券停用、超限…） | `FAILED`(错误码, 文案) + 失败通知 |
| 冷却 / 未预热 / 尚未开始 / 限流 / 5xx / 网络 / 超时 / panic | 瞬态：退避重试（`RetryBackoff × 2^n`，封顶 1 分钟），第 `MaxRetry` 次仍失败 → `FAILED`（通知文案带“已达重试上限”） |

调度器自身被取消（关闭）时，已调用入口但结果未知的预约保持 `FIRING`，由回收接手；
入口**已成功受理**的结果用脱离 ctx 的短超时写回，保证“已受理”这个事实不丢。

## 结果回写（`Service.OnSeckillResult`）

按 `request_id` 找到预约，`QUEUED → SUCCESS`（记录 `order_id`）或 `→ FAILED`（记录失败码与原因），
**在同一事务里写通知**；预约已是终态（重复投递、并发投递）不再迁移、不再写通知；找不到预约（手动秒杀）返回 nil。
结果事件可能先于调度器的 `QUEUED` 回写到达：此时按 (user, coupon) 兜底匹配仍在 `FIRING` 且没有 `request_id` 的预约
（`lockForResult` 用一条语句同时表达两个分支，调度器同时改这一行时也不会漏）。

## 配置（环境变量，加载见 `Config`）

| 变量 | 默认 | 说明 |
|---|---|---|
| `CK_ORDER_RESERVATION_ENABLED` | `true` | 关闭则 `Run` 空转到 ctx 取消（HTTP 接口不受影响） |
| `CK_ORDER_RESERVATION_SCAN_INTERVAL` | `1s` | 调度扫描间隔 |
| `CK_ORDER_RESERVATION_BATCH_SIZE` | `50` | 每次认领/回收/回查的最大条数；认领满一批立即再认领直到清空 |
| `CK_ORDER_RESERVATION_CONCURRENCY` | `4` | 单实例并发调用入口的 worker 数 |
| `CK_ORDER_RESERVATION_MAX_RETRY` | `3` | 单条预约调用入口的最大尝试次数（含首次，沿用 Java `max-retry` 语义） |
| `CK_ORDER_RESERVATION_RETRY_BACKOFF` | `2s` | 首次重试前的退避，之后翻倍，封顶 1 分钟 |
| `CK_ORDER_RESERVATION_ENTER_TIMEOUT` | `10s` | 单次调用入口超时，必须小于 `STALE_FIRING_TIMEOUT` |
| `CK_ORDER_RESERVATION_STALE_FIRING_TIMEOUT` | `2m` | `FIRING` 最长停留时间，超过视为认领方崩溃 |
| `CK_ORDER_RESERVATION_QUEUED_SYNC_AFTER` | `1m` | `QUEUED` 无结果多久后回查（仅配置了 `ResultReader` 时） |

`reservation.Config` 带 `env` tag，order 服务把它内嵌进自己的配置结构体即可由 `config.Load` 加载；
`NewScheduler` 还会调用 `Config.Validate()` 校验跨字段约束。

## HTTP 接口

挂载：`reservation.RegisterRoutes(r, svc)`，完整路径 `/api/v1/order/reservations`。
`r` 必须已挂 `httpx.RequireGatewayIdentity`（漏挂时处理器取不到身份，一律 401，失败即关闭）。
所有 ID 输出为字符串；时间为 RFC3339（带时区）；未发生的字段为 `null`。
别人的预约与不存在的预约不可区分，一律 404。

| 方法 路径 | 说明 | 成功 data | 失败 |
|---|---|---|---|
| `POST /` | 创建 `{"couponId":"1001"}`（字符串或数字） | 预约对象 | 400 参数/非秒杀券/未配置时间窗/已开售；404 券不存在(10008)；409 券停用(10014)/活动已结束(10007)/重复预约(10010)；503 券服务不可用(10013) |
| `DELETE /:id` | 取消（仅属主、仅 `PENDING`） | `true` | 404；409 当前不是 `PENDING`（消息含当前状态） |
| `GET /mine?page=1&size=100` | 我的预约，最新在前；`size` 默认且最大 100（沿用 Java `LIMIT 100`，前端一次取全） | `{"items":[…],"total":N,"page":P,"size":S}` | 400 参数越界 |
| `GET /:id` | 详情 | 预约对象 | 404 |

`failReason` 在终态失败时是 `events.Reason` 词表的原因码（`OUT_OF_STOCK`、`REPEAT`、`COUPON_DISABLED`、
`COUPON_NOT_FOUND`、`LIMIT_EXCEEDED`、`TIMEOUT`、`SYSTEM_ERROR`…），前端据此映射中文；`failCode` 是对应的业务错误码。
`EXPIRED` 没有对应的原因码，`failReason` 是文案“活动已结束仍未触发”。给用户的中文说明在站内通知里。

预约对象（字段名与前端 `SeckillReservation` 一致；时间字段为 `createdAt`/`updatedAt`；Java 的 `version` 已去掉）：

```json
{
  "code": 200, "message": "ok",
  "data": {
    "id": "1859263489312768001",
    "userId": "42",
    "couponId": "1001",
    "status": "QUEUED",
    "reserveAt": "2026-11-06T09:00:00.123456Z",
    "triggerAt": "2026-11-06T10:00:00Z",
    "firedAt": "2026-11-06T10:00:00.050Z",
    "requestId": "9f2c…",
    "orderId": null,
    "failCode": null,
    "failReason": null,
    "retryCount": 0,
    "createdAt": "2026-11-06T09:00:00.123456Z",
    "updatedAt": "2026-11-06T10:00:00.081Z"
  }
}
```

失败信封示例（重复预约）：

```json
{"code": 10010, "message": "您已预约过该券，请勿重复预约", "data": null, "requestId": "…"}
```

## 装配（order 服务）

```go
pool := …                                   // *pgxpool.Pool
resSvc := reservation.NewService(pool, idGen, couponLookup)
sched, err := reservation.NewScheduler(resSvc, seckillAdapter, cfg.Reservation /*, reservation.WithResultReader(rr)*/)

reservation.RegisterRoutes(authed, resSvc)  // authed 已挂 httpx.RequireGatewayIdentity
// app.Options.Workers: {Name: "reservation-scheduler", Run: sched.Run}
// seckill_order_result 消费者里：resSvc.OnSeckillResult(ctx, reservation.ResultEvent{…})
```

适配器（接口都定义在本包）：

```go
// CouponLookup：internalapi.CouponClient 的适配
type couponLookup struct{ c internalapi.CouponGetter }

func (l couponLookup) Coupon(ctx context.Context, id int64) (reservation.Coupon, error) {
    info, err := l.c.GetCoupon(ctx, id) // 券不存在时返回 apperr.CouponNotFound
    if err != nil {
        return reservation.Coupon{}, err
    }
    return reservation.Coupon{ID: info.ID, Type: info.Type, Status: info.Status,
        SeckillStartAt: info.SeckillStartAt, SeckillEndAt: info.SeckillEndAt}, nil
}

// Seckiller：order-core 的秒杀入口，返回 requestId；错误原样返回 apperr
type seckillAdapter struct{ s *seckill.Service }
```

`Seckiller.Enter` 的契约（详见 `model.go`）：成功返回 `requestID`；用户已占位返回 `apperr.RepeatSeckill`；
活动已结束返回 `apperr.ActivityEnded`；其他 4xx 业务错误视为终态；冷却/未预热/尚未开始/繁忙/网络视为瞬态。

## 不变式与测试

| 不变式 | 证明 |
|---|---|
| 状态只能沿 `transitions` 迁移，终态不变 | `TestCanTransitionMatchesWrittenTable`、`TestMoveEnforcesEveryTransitionInDatabase`（49 组合） |
| 同用户同券至多一个进行中预约（并发创建只成功一个） | `TestCreateRejectsDuplicateActiveReservation`、`TestCreateConcurrentOnlyOneWins` |
| 50 条到点预约、4 个调度实例并发，每条恰好触发一次 | `TestSchedulerFiftyReservationsFiredExactlyOnceByFourInstances`（批量 50 与 7 两种争抢形态） |
| 认领后崩溃：超时前不抢、超时后回收并重试；入口已受理的崩溃映射 `RepeatSeckill` | `TestSchedulerCrashAfterClaim…`、`TestSchedulerCrashAfterEnterBeforeWritebackMapsRepeatSeckill` |
| 被回收后迟到的写回不覆盖新一轮（fence） | `TestSchedulerLateWritebackAfterRecoveryIsFenced`、`TestSchedulerDoesNotEnterForRowsReclaimedWhileWaitingInBatch` |
| 瞬态失败退避重试，第 `MaxRetry` 次后 `FAILED` | `TestSchedulerRetriesTransientErrorsWithBackoffThenFails`（7 种瞬态错误） |
| 结果事件重复/并发投递只产生一条通知 | `TestOnSeckillResultRedeliveryIsIdempotent` |
| 结果事件先于 `QUEUED` 回写到达不丢 | `TestOnSeckillResultBeforeQueuedWritebackMatchesFiringReservation`、`TestSchedulerResultEventBeforeQueuedWritebackWins` |

测试用真实 PostgreSQL（`dbxtest`，`CK_TEST_PG_DSN` 或内嵌 PG）+ 假 `Seckiller`/`CouponLookup`/`ResultReader` + `testkit.Clock` 假时钟。
