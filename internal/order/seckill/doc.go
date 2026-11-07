// Package seckill 实现 order 服务的秒杀状态机（设计 §9）：入口、建单消费者、超时清扫器与回补助手。
//
// # 状态机
//
//	POST /seckill ─ 校验券 ─ enter.lua ─ 发布 Kafka ─▶ 202 QUEUED
//	                          │              └ 失败：rollback.lua ─▶ 503
//	                          ▼
//	      请求哈希 ck:sk:req:{rid}:  QUEUED ─▶ PROCESSING ─▶ SUCCESS | FAILED
//	                                   └────────────────────────▶ FAILED（sweeper 超时 / 入队失败）
//
// SUCCESS 与 FAILED 是终态，终态一旦写入不得再改：所有状态变更都经 Lua 做 compare-and-set
// （advance.lua 推进 PROCESSING/SUCCESS，rollback.lua 置 FAILED 并回补）。
//
// # 不变式（simulation_test.go 逐条验证）
//
//  1. 每个 requestId 至多一次库存扣减生效、至多一个订单：coupon 的账本按 requestId 幂等，
//     订单靠 ON CONFLICT (user_id, request_id) DO NOTHING 幂等。
//  2. 库存守恒：Redis 预扣计数 + 在途请求数 + 成功订单数 = 初始库存（计数永不为负）。
//     回补由 rollback.lua 的 rolledBack 标记保证最多一次，取消订单与失败终态共用它。
//  3. 消费者对任何一步的重放都幂等：重复投递、崩溃后重投、coupon 响应丢失都不会多扣、多建单。
//  4. transient 错误（internalapi.IsTransient）只重试、绝不补偿：同一 requestId 的重试是安全的，
//     重试耗尽后进死信，由 sweeper 在超时后统一收尾。
//  5. 迟到的订单不会让库存多出一份：若 sweeper/入队失败已把请求置为 FAILED，之后才落库的订单
//     会在置 SUCCESS 时被 CAS 拒绝，并被取消、库存被归还（lateCleanup）。
//
// # 规格补充
//
//   - 请求哈希多一个 rolledBack 字段（回补幂等标记），keys 契约里的字段表只列了读者关心的字段。
//   - 入口失败（校验、Lua 拒绝、入队失败）以错误返回给调用者，不触发 ResultHook；
//     ResultHook 只在异步终态（消费者落单/失败、sweeper 收尾）后调用，且只在本次调用真正完成了
//     状态迁移时调用一次（重放与被他人抢先终态的情况不会重复通知）。
//   - ck:cpn:meta:{id} 的内容约定为 internalapi.CouponInfo 的 JSON，coupon 服务写缓存时必须使用同一形状。
package seckill
