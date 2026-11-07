package events

import (
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// Status 是秒杀请求在 Redis 请求哈希（keys.SeckillRequest 的 status 字段）里的状态。
//
// 状态机：QUEUED -> PROCESSING -> SUCCESS | FAILED；QUEUED 也可以直接转 FAILED
// （入队失败回滚、sweeper 超时）。SUCCESS 与 FAILED 是终态，终态一旦写入不得再改。
type Status string

// 请求状态词表，大小写敏感。
const (
	// StatusQueued 表示已通过 Lua 预扣并发布到 Kafka，等待消费。
	StatusQueued Status = "QUEUED"
	// StatusProcessing 表示消费者已领取，正在扣库存/落单。
	StatusProcessing Status = "PROCESSING"
	// StatusSuccess 表示订单已落库，哈希里有 orderId。
	StatusSuccess Status = "SUCCESS"
	// StatusFailed 表示请求失败，哈希里有 reason。
	StatusFailed Status = "FAILED"
)

// Valid 判断是否为词表内的状态。
func (s Status) Valid() bool {
	switch s {
	case StatusQueued, StatusProcessing, StatusSuccess, StatusFailed:
		return true
	}
	return false
}

// Terminal 判断是否为终态（SUCCESS 或 FAILED）。
func (s Status) Terminal() bool {
	return s == StatusSuccess || s == StatusFailed
}

// Reason 是 FAILED 状态的失败原因，写在请求哈希的 reason 字段，并经 /seckill/result 回给前端。
type Reason string

// 失败原因词表，大小写敏感。
const (
	// ReasonOutOfStock 表示 coupon 服务所有库存桶均已售罄。
	ReasonOutOfStock Reason = "OUT_OF_STOCK"
	// ReasonRepeat 表示用户已持有该券（违反 (user_id, coupon_id) 部分唯一索引）。
	ReasonRepeat Reason = "REPEAT"
	// ReasonTimeout 表示 sweeper 发现请求超时未完成并已回滚。
	ReasonTimeout Reason = "TIMEOUT"
	// ReasonCouponDisabled 表示券在处理期间被停用。
	ReasonCouponDisabled Reason = "COUPON_DISABLED"
	// ReasonCouponNotFound 表示券在处理期间被删除。
	ReasonCouponNotFound Reason = "COUPON_NOT_FOUND"
	// ReasonLimitExceeded 表示超过单用户领取上限。
	ReasonLimitExceeded Reason = "LIMIT_EXCEEDED"
	// ReasonEnqueueFailed 表示发布 Kafka 失败，预扣已回滚。
	ReasonEnqueueFailed Reason = "ENQUEUE_FAILED"
	// ReasonSystemError 表示其他不可恢复的内部错误。
	ReasonSystemError Reason = "SYSTEM_ERROR"
)

// Valid 判断是否为词表内的失败原因。
func (r Reason) Valid() bool {
	switch r {
	case ReasonOutOfStock, ReasonRepeat, ReasonTimeout, ReasonCouponDisabled,
		ReasonCouponNotFound, ReasonLimitExceeded, ReasonEnqueueFailed, ReasonSystemError:
		return true
	}
	return false
}

// ReasonForError 把处理链路上的错误归类为失败原因；不认识的错误一律归为 ReasonSystemError。
func ReasonForError(err error) Reason {
	switch apperr.CodeOf(err) {
	case apperr.CodeOutOfStock:
		return ReasonOutOfStock
	case apperr.CodeRepeatSeckill:
		return ReasonRepeat
	case apperr.CodeCouponLimitExceeded:
		return ReasonLimitExceeded
	case apperr.CodeCouponDisabled:
		return ReasonCouponDisabled
	case apperr.CodeCouponNotFound:
		return ReasonCouponNotFound
	default:
		return ReasonSystemError
	}
}

// AppError 返回失败原因对应的业务错误，供需要把终态当作错误上抛的调用方使用
// （例如预约到点代抢后把失败原因写进通知）。TIMEOUT/ENQUEUE_FAILED 表现为系统繁忙，
// 未知原因表现为系统错误。
func (r Reason) AppError() *apperr.Error {
	switch r {
	case ReasonOutOfStock:
		return apperr.OutOfStock
	case ReasonRepeat:
		return apperr.RepeatSeckill
	case ReasonLimitExceeded:
		return apperr.CouponLimitExceeded
	case ReasonCouponDisabled:
		return apperr.CouponDisabled
	case ReasonCouponNotFound:
		return apperr.CouponNotFound
	case ReasonTimeout, ReasonEnqueueFailed:
		return apperr.SystemBusy
	default:
		return apperr.SysError
	}
}

// SeckillAccepted 是 POST /api/v1/order/seckill 受理成功（HTTP 202）时 data 的内容。
type SeckillAccepted struct {
	// RequestID 供前端轮询 /seckill/result。
	RequestID string `json:"requestId"`
	// Status 恒为 StatusQueued。
	Status Status `json:"status"`
}

// SeckillResult 是 GET /api/v1/order/seckill/result 的 data。
// 非终态只有 Status；SUCCESS 带 OrderID；FAILED 带 Reason。
type SeckillResult struct {
	// Status 是当前请求状态。
	Status Status `json:"status"`
	// OrderID 仅在 SUCCESS 时出现，以字符串编码。
	OrderID int64 `json:"orderId,string,omitempty"`
	// Reason 仅在 FAILED 时出现。
	Reason Reason `json:"reason,omitempty"`
}
