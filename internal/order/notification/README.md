# internal/order/notification —— 站内通知

`ord.notifications` 的读写与 HTTP 接口。写入方：预约流程（`internal/order/reservation`，与状态迁移同事务）
与秒杀建单（order-core，类型 `SECKILL_SUCCESS`，在建单事务里写）；读取方：前端铃铛。

## 通知类型（`Type*` 常量）

| 常量 | 值 | 写入方 |
|---|---|---|
| `TypeSeckillSuccess` | `SECKILL_SUCCESS` | order-core（建单事务内） |
| `TypeReservationSuccess` | `RESERVATION_SUCCESS` | reservation |
| `TypeReservationFailed` | `RESERVATION_FAILED` | reservation |
| `TypeReservationExpired` | `RESERVATION_EXPIRED` | reservation |

## 写入：`Insert`

```go
err := notification.Insert(ctx, tx /* pgx.Tx 或 *pgxpool.Pool */, notification.Notification{
    ID: idGen.Next(), UserID: userID, Type: notification.TypeSeckillSuccess,
    Title: "秒杀成功", Content: "…", RefID: &orderID,
})
```

`ID`、`UserID`、`Type`、`Title` 必填（否则返回错误，不写库）；`CreatedAt` 为零值时用数据库 `now()`；
`RefID` 为 nil 表示无关联。幂等性由调用方负责（通知只在状态迁移真正发生时写，重放不会重复）。

## HTTP 接口

挂载：`notification.RegisterRoutes(r, notification.NewService(pool))`，完整路径 `/api/v1/order/notifications`。
`r` 必须已挂 `httpx.RequireGatewayIdentity`（漏挂时一律 401）。所有操作限定为调用者本人：
SQL 带 `user_id` 谓词（命中 `(user_id, created_at DESC)` 与未读部分索引）；
别人的通知与不存在的通知不可区分，一律 404。

| 方法 路径 | 说明 | 成功 data |
|---|---|---|
| `GET /mine?page=1&size=20` | 我的通知，最新在前；`size` 默认 20、最大 50（也接受旧参数 `limit`，等价于 `size`） | `{"items":[…],"total":N,"page":P,"size":S}` |
| `GET /unread-count` | 未读数 | `{"count":3}` |
| `POST /:id/read` | 单条已读；重复调用同样 200 | `true`（别人的/不存在的 404） |
| `POST /read-all` | 全部已读 | 本次**新标记**的条数（再调为 0） |

通知对象（字段名与前端 `UserNotification` 一致）：

```json
{
  "code": 200, "message": "ok",
  "data": {
    "items": [
      {
        "id": "1859263489312768002",
        "userId": "42",
        "type": "RESERVATION_SUCCESS",
        "title": "预约帮抢成功",
        "content": "券 1001 已抢到，订单 1859263489312768003",
        "refId": "1859263489312768001",
        "readFlag": false,
        "createdAt": "2026-11-06T10:00:00.081Z"
      }
    ],
    "total": 1, "page": 1, "size": 20
  }
}
```

## 与 Java 的差异

- 列表由“返回数组”改为设计 §4 的分页信封（前端需读取 `data.items`）。
- 不存在/他人的通知由 `200 + INVALID_REQ` 改为真实的 `404`。
- 通知写入失败**不再被吞掉**：预约相关通知与状态迁移同事务，失败则整体回滚，由重试/回收重做（Java 是吞异常，会丢通知）。
