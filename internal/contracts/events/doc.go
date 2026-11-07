// Package events 定义跨服务的 Kafka 载荷、主题常量，以及秒杀请求的状态与失败原因词表。
//
// # Kafka 契约
//
//	主题   seckill_order_create       生产者 order（秒杀入口）  消费者 order（建单消费者）
//	死信   seckill_order_create_dlt   消息体与原主题相同，由消费者在重试耗尽或载荷非法时写入
//	消费组 seckill-order-create-group
//	分区键 userId 的十进制字符串（SeckillOrderCreateKey）：同一用户的消息有序，落同一分区
//
// 载荷是 JSON 对象 SeckillOrderCreate{requestId, userId, couponId, enqueuedAtMs}。
// 约定：
//   - 所有 ID（userId、couponId）一律编码为 JSON 字符串（Go 里是 int64 + `json:",string"`），
//     避免雪花 ID 在 JS/JSON 数字里丢精度。本模块全部契约（keys/events/internalapi）统一用这一种做法。
//   - 解码忽略未知字段（向前兼容：生产者可以先加字段）。
//   - 生产者与消费者都必须经 MarshalSeckillOrderCreate / UnmarshalSeckillOrderCreate，
//     它们会拒绝非正 ID、空或含非法字符的 requestId、非正的 enqueuedAtMs。
//     校验失败的错误满足 errors.Is(err, ErrInvalidEvent)，消费者应直接写死信而非重试。
//   - 与设计 §9 的差异：时间字段命名为 enqueuedAtMs（毫秒整数），而非 enqueuedAt。
//
// # 请求状态词表
//
// Status（QUEUED/PROCESSING/SUCCESS/FAILED）与 Reason（OUT_OF_STOCK/REPEAT/TIMEOUT/...）是
// Redis 请求哈希（keys.SeckillRequest）里 status、reason 字段的取值，同时也是
// GET /api/v1/order/seckill/result 响应（SeckillResult）的取值。SeckillAccepted 是
// POST /api/v1/order/seckill 返回 202 时 data 的形状。ReasonForError 把链路上的 apperr
// 归类成 Reason，Reason.AppError 反向映射。
package events
