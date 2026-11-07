// Package keys 是 Redis 键与相关 TTL 的唯一定义处（设计文档 §8）。
//
// 所有键统一以 "ck:" 为前缀，由纯函数构造，不访问 Redis。order、coupon、gateway、connector
// 必须通过本包取键，禁止手写字符串拼接——键空间是跨服务契约，Helm 的排障脚本与 Lua 脚本的
// KEYS 顺序也依赖这里的格式。
//
// 键一览（{...} 为参数；ID 一律是十进制 int64）：
//
//	ck:stock:{couponId}                 int   秒杀预扣计数器，由 coupon 服务预热/同步写入；键不存在 = 未预热
//	ck:sk:user:{couponId}:{userId}      str   用户已占位标记（值=requestId），TTL SeckillMarkerTTL
//	ck:sk:cool:{couponId}:{userId}      str   冷却标记（可选）
//	ck:sk:req:{requestId}               hash  请求状态，字段见 ReqField*，TTL SeckillRequestTTL
//	ck:sk:inflight                      zset  member=requestId，score=入队毫秒时间戳
//	ck:cpn:meta:{couponId}              json  券元数据缓存，TTL CouponMetaTTL
//	ck:sess:{jti}                       str   登录会话（网关校验吊销）
//	ck:lock:{name}                      str   分布式锁
//
// 单实例/主从 Redis 下 Lua 脚本可以任意组合这些键；如果将来切到 Redis Cluster，需要先为
// 同一次 Lua 调用涉及的键引入 hash tag，届时只改本包。
//
// 请求 ID 必须来自受信任的生成器（UUID/hex），不要把用户输入直接当作 requestId 传入；
// events.ValidRequestID 给出了允许的字符集。
package keys
