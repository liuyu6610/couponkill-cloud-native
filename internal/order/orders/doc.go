// Package orders 是 order 服务的订单域：订单行的读写原语（store.go）、
// 普通券下单 / 取消 / 查询 / 管理员发券的业务规则（service.go）以及对应的 HTTP 处理器（handler.go）。
//
// # 不变式
//
//   - 同一 (user_id, request_id) 至多一行订单：所有写入都经 Insert 的
//     ON CONFLICT (user_id, request_id) DO NOTHING，重放不会产生第二单。
//   - 同一用户对同一张券至多一笔未取消订单：由部分唯一索引 uk_orders_user_coupon_active 保证。
//     分区表上的唯一冲突会报告分区自己的索引名，所以代码从不按约束名匹配，
//     预期内的重复靠 ON CONFLICT 吸收，其余唯一冲突（dbx.IsUniqueViolation(err, "")）即「已持有」。
//   - 取消只做 status 1→4 的单条件更新：并发取消只有一个能赢，赢的人负责回补库存。
//   - 所有已知用户的查询都带 user_id（分区裁剪）；只有管理员按 id 点查与管理端列表允许跨分区。
//
// 包内的 SQL 全部使用 "ord." 限定名，不依赖连接的 search_path。
package orders
