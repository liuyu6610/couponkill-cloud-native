package keys

import (
	"strconv"
	"time"
)

// Prefix 是所有 Redis 键的统一前缀。
const Prefix = "ck:"

// TTL 常量属于契约：order 写入占位标记/请求哈希、coupon 写入元数据缓存时都必须使用它们。
const (
	// SeckillMarkerTTL 是用户占位标记（SeckillUser）的过期时间，用于防重复秒杀。
	SeckillMarkerTTL = 24 * time.Hour
	// SeckillRequestTTL 是请求状态哈希（SeckillRequest）的过期时间。
	SeckillRequestTTL = 24 * time.Hour
	// CouponMetaTTL 是券元数据缓存（CouponMeta）的过期时间；coupon 服务变更券时还会主动删除。
	CouponMetaTTL = 5 * time.Minute
)

// 请求状态哈希（SeckillRequest）的字段名。
const (
	// ReqFieldStatus 是请求状态，取值见 events.Status。
	ReqFieldStatus = "status"
	// ReqFieldUserID 是请求属主，/seckill/result 据此校验归属。
	ReqFieldUserID = "userId"
	// ReqFieldCouponID 是请求对应的券 ID。
	ReqFieldCouponID = "couponId"
	// ReqFieldOrderID 是成功后的订单 ID，仅 SUCCESS 时存在。
	ReqFieldOrderID = "orderId"
	// ReqFieldReason 是失败原因，取值见 events.Reason，仅 FAILED 时存在。
	ReqFieldReason = "reason"
	// ReqFieldUpdatedAt 是最近一次状态变更的毫秒时间戳。
	ReqFieldUpdatedAt = "updatedAt"
)

// Stock 返回秒杀预扣计数器键 ck:stock:{couponId}。
func Stock(couponID int64) string {
	return Prefix + "stock:" + id(couponID)
}

// SeckillUser 返回用户占位标记键 ck:sk:user:{couponId}:{userId}。
func SeckillUser(couponID, userID int64) string {
	return Prefix + "sk:user:" + id(couponID) + ":" + id(userID)
}

// SeckillCooldown 返回用户冷却键 ck:sk:cool:{couponId}:{userId}。
func SeckillCooldown(couponID, userID int64) string {
	return Prefix + "sk:cool:" + id(couponID) + ":" + id(userID)
}

// SeckillRequest 返回请求状态哈希键 ck:sk:req:{requestId}。
func SeckillRequest(requestID string) string {
	return Prefix + "sk:req:" + requestID
}

// SeckillInflight 返回在途请求有序集合键 ck:sk:inflight。
func SeckillInflight() string {
	return Prefix + "sk:inflight"
}

// CouponMeta 返回券元数据缓存键 ck:cpn:meta:{couponId}。
func CouponMeta(couponID int64) string {
	return Prefix + "cpn:meta:" + id(couponID)
}

// Session 返回登录会话键 ck:sess:{jti}。
func Session(jti string) string {
	return Prefix + "sess:" + jti
}

// Lock 返回分布式锁键 ck:lock:{name}。
func Lock(name string) string {
	return Prefix + "lock:" + name
}

func id(v int64) string {
	return strconv.FormatInt(v, 10)
}
