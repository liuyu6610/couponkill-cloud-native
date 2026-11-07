package reservation

import (
	"context"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/notification"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
)

// 券类型与状态取值（与 internalapi.CouponInfo 的 Type/Status 语义一致；本包不依赖 internalapi，由装配方适配）。
const (
	// CouponTypeNormal 是常驻券。
	CouponTypeNormal = 1
	// CouponTypeSeckill 是秒杀券，只有它支持预约帮抢。
	CouponTypeSeckill = 2
	// CouponStatusDisabled 是停用。
	CouponStatusDisabled = 0
	// CouponStatusEnabled 是启用。
	CouponStatusEnabled = 1
)

// DBTX 是读写 SQL 需要的最小能力；*pgxpool.Pool 与 pgx.Tx 都满足它。
type DBTX = notification.DBTX

// DB 是 Service 需要的数据库能力：执行 SQL，并能开启事务。*pgxpool.Pool 满足它。
type DB interface {
	DBTX
	dbx.TxBeginner
}

// IDGenerator 生成预约与通知的雪花 ID；*idgen.Generator 满足它。
type IDGenerator interface {
	Next() int64
}

// Coupon 是创建预约时需要的券事实，由 CouponLookup 提供。
type Coupon struct {
	// ID 是券 ID。
	ID int64
	// Type 见 CouponType*。
	Type int
	// Status 见 CouponStatus*。
	Status int
	// SeckillStartAt 是开售时间；没有配置时间窗为 nil。
	SeckillStartAt *time.Time
	// SeckillEndAt 是活动结束时间；没有配置时间窗为 nil。
	SeckillEndAt *time.Time
}

// CouponLookup 取券事实。券不存在必须返回 apperr.CouponNotFound（或能被 errors.Is 识别为它的错误）；
// 其他失败（网络、超时）返回任意 error，Service 会统一表现为“优惠券服务暂不可用”。
// 装配方用 internalapi.CouponClient.GetCoupon 做适配。
type CouponLookup interface {
	Coupon(ctx context.Context, couponID int64) (Coupon, error)
}

// Seckiller 是调度器到点代用户发起秒杀的入口，装配方用 order-core 的秒杀服务适配。
//
// 语义要求：成功返回 requestID（已入队）；用户已占位时返回 apperr.RepeatSeckill；
// 活动已结束返回 apperr.ActivityEnded；其他 apperr 终态错误（库存不足、券停用等）原样返回；
// 瞬态错误（冷却、未预热、尚未开始、系统繁忙、网络）可以返回 apperr 或普通 error，调度器会退避重试。
type Seckiller interface {
	Enter(ctx context.Context, userID, couponID int64) (requestID string, err error)
}

// ResultReader 是可选的“结果回查”能力：按 requestID 读取秒杀请求的终态。
// 配置后，调度器会对长时间停在 QUEUED 的预约主动回查，兜底结果事件丢失（例如进了死信）。
// 请求还没有终态时返回 found=false。
type ResultReader interface {
	Result(ctx context.Context, requestID string) (ev ResultEvent, found bool, err error)
}

// ResultEvent 是秒杀建单结果，由装配方从 seckill_order_result 消费者（或 Redis 请求哈希）适配而来。
type ResultEvent struct {
	// RequestID 是秒杀请求号（预约成功入队时记录在预约上）。
	RequestID string
	// UserID 与 CouponID 用于在结果早于 QUEUED 回写到达时兜底匹配 FIRING 中的预约。
	UserID   int64
	CouponID int64
	// Success 为 true 表示订单已生成。
	Success bool
	// OrderID 是订单号，仅 Success 时有意义（可为 0）。
	OrderID int64
	// Reason 是失败原因，通常是 events.Reason 的取值（如 OUT_OF_STOCK）；为空或未知时按系统错误处理。
	Reason string
}

// Reservation 是一条预约帮抢记录，JSON 字段名与前端 SeckillReservation 类型一致（ID 一律输出为字符串）。
type Reservation struct {
	// ID 是预约 ID。
	ID int64 `json:"id,string"`
	// UserID 是预约人。
	UserID int64 `json:"userId,string"`
	// CouponID 是被预约的秒杀券。
	CouponID int64 `json:"couponId,string"`
	// Status 见 Status*。
	Status Status `json:"status"`
	// ReserveAt 是预约时间。
	ReserveAt time.Time `json:"reserveAt"`
	// TriggerAt 是下一次自动触发的时间（首次等于券的开售时间，退避重试时后移）。
	TriggerAt *time.Time `json:"triggerAt"`
	// FiredAt 是最近一次被调度器认领的时间。
	FiredAt *time.Time `json:"firedAt"`
	// RequestID 是秒杀请求号，入队后才有。
	RequestID *string `json:"requestId"`
	// OrderID 是抢到的订单号，SUCCESS 且已知订单时才有。
	OrderID *int64 `json:"orderId,string"`
	// FailCode 是失败的业务错误码（FAILED/EXPIRED 时）。
	FailCode *int `json:"failCode"`
	// FailReason 是失败原因文案。
	FailReason *string `json:"failReason"`
	// RetryCount 是已消耗的重试次数。
	RetryCount int `json:"retryCount"`
	// CreatedAt 是创建时间。
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time `json:"updatedAt"`
}
