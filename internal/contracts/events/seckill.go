package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Kafka 主题与消费组。名称与 Helm/KEDA 配置一致，改动属于契约变更。
const (
	// TopicSeckillOrderCreate 是秒杀受理后的建单命令主题，分区键为 userId。
	TopicSeckillOrderCreate = "seckill_order_create"
	// TopicSeckillOrderCreateDLT 是建单命令的死信主题，消息体与原主题相同。
	TopicSeckillOrderCreateDLT = "seckill_order_create_dlt"
	// GroupSeckillOrderCreate 是 order 服务消费建单命令的消费组。
	GroupSeckillOrderCreate = "seckill-order-create-group"
)

// maxRequestIDLen 是 requestId 允许的最大长度。
const maxRequestIDLen = 128

// ErrInvalidEvent 表示事件载荷不合法（无法解码或校验失败）。消费者遇到它应当直接送入死信，
// 而不是重试：重放同一条坏消息永远不会成功。
var ErrInvalidEvent = errors.New("events: invalid event")

// SeckillOrderCreate 是 order 服务在 Lua 预扣成功后发布的建单命令（设计 §9 步骤 3）。
//
// 线上格式（JSON）：
//
//	{"requestId":"...","userId":"123","couponId":"1001","enqueuedAtMs":1760000000123}
//
// ID 以字符串编码，避免雪花 ID 超过 2^53 后在 JS/JSON 数字里丢精度。解码时忽略未知字段，
// 以便生产者先于消费者升级。
type SeckillOrderCreate struct {
	// RequestID 是一次秒杀请求的全局唯一标识，也是库存账本与订单的幂等键。
	RequestID string `json:"requestId"`
	// UserID 是抢券用户。
	UserID int64 `json:"userId,string"`
	// CouponID 是被抢的券。
	CouponID int64 `json:"couponId,string"`
	// EnqueuedAtMs 是受理时的 Unix 毫秒时间戳，与 ck:sk:inflight 的 score 一致。
	EnqueuedAtMs int64 `json:"enqueuedAtMs"`
}

// NewSeckillOrderCreate 用给定的受理时间构造建单命令。
func NewSeckillOrderCreate(requestID string, userID, couponID int64, enqueuedAt time.Time) SeckillOrderCreate {
	return SeckillOrderCreate{
		RequestID:    requestID,
		UserID:       userID,
		CouponID:     couponID,
		EnqueuedAtMs: enqueuedAt.UnixMilli(),
	}
}

// Validate 校验命令：requestId 合法、两个 ID 为正数、受理时间为正数。
// 返回的错误满足 errors.Is(err, ErrInvalidEvent)。
func (m SeckillOrderCreate) Validate() error {
	switch {
	case !ValidRequestID(m.RequestID):
		return fmt.Errorf("%w: requestId %q is empty or malformed", ErrInvalidEvent, m.RequestID)
	case m.UserID <= 0:
		return fmt.Errorf("%w: userId must be positive, got %d", ErrInvalidEvent, m.UserID)
	case m.CouponID <= 0:
		return fmt.Errorf("%w: couponId must be positive, got %d", ErrInvalidEvent, m.CouponID)
	case m.EnqueuedAtMs <= 0:
		return fmt.Errorf("%w: enqueuedAtMs must be positive, got %d", ErrInvalidEvent, m.EnqueuedAtMs)
	}
	return nil
}

// Key 返回该命令的 Kafka 分区键（userId 的十进制字符串）。
func (m SeckillOrderCreate) Key() []byte {
	return SeckillOrderCreateKey(m.UserID)
}

// SeckillOrderCreateKey 返回建单命令的分区键：同一用户的消息落在同一分区，保证按用户有序，
// 也避免同一用户的并发重复请求被不同消费者同时处理。
func SeckillOrderCreateKey(userID int64) []byte {
	return []byte(strconv.FormatInt(userID, 10))
}

// MarshalSeckillOrderCreate 校验并编码建单命令。
func MarshalSeckillOrderCreate(m SeckillOrderCreate) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("events: marshal seckill order create: %w", err)
	}
	return data, nil
}

// UnmarshalSeckillOrderCreate 解码并校验建单命令；任何解码或校验失败都返回
// 满足 errors.Is(err, ErrInvalidEvent) 的错误。
func UnmarshalSeckillOrderCreate(data []byte) (SeckillOrderCreate, error) {
	var m SeckillOrderCreate
	if err := json.Unmarshal(data, &m); err != nil {
		return SeckillOrderCreate{}, fmt.Errorf("%w: decode: %v", ErrInvalidEvent, err)
	}
	if err := m.Validate(); err != nil {
		return SeckillOrderCreate{}, err
	}
	return m, nil
}

// ValidRequestID 判断 requestId 是否合法：1 到 128 个字符，只含字母、数字和 "._:-"。
// 限制字符集是为了让 requestId 可以安全地拼进 Redis 键、日志和 URL，而不会被注入分隔符。
func ValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == ':', c == '-':
		default:
			return false
		}
	}
	return true
}
