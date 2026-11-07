package internalapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/events"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 内部端点路径。*Route 常量使用 gin 的 ":id" 语法，供服务端注册路由；客户端用 CouponPath/UserPath。
const (
	// PathStockDeduct 是 coupon 的库存扣减端点（POST，按 requestId 幂等）。
	PathStockDeduct = "/internal/v1/stock/deduct"
	// PathStockRelease 是 coupon 的库存归还端点（POST，按 requestId 幂等）。
	PathStockRelease = "/internal/v1/stock/release"
	// PathStockSync 是 coupon 的库存同步端点（POST，按 SyncRequest.Mode 调整可用量）。
	PathStockSync = "/internal/v1/stock/sync"
	// PathCouponRoute 是 coupon 的券详情路由模板（GET）。
	PathCouponRoute = "/internal/v1/coupons/:id"
	// PathOrderGrant 是 order 的发券建单端点（POST，按 requestId 幂等）。
	PathOrderGrant = "/internal/v1/orders/grant"
	// PathUserRoute 是 user 的用户详情路由模板（GET）。
	PathUserRoute = "/internal/v1/users/:id"
)

// 内部调用使用的请求头。
const (
	// HeaderInternalToken 携带服务间共享令牌（CK_INTERNAL_TOKEN）。
	HeaderInternalToken = "X-Internal-Token"
	// HeaderRequestID 在服务间传播请求 ID，便于日志关联。
	HeaderRequestID = "X-Request-Id"
)

// 券类型与状态，取值与 coupons 表、前端 types/api.ts 一致。
const (
	// CouponTypeNormal 是常驻券。
	CouponTypeNormal = 1
	// CouponTypeSeckill 是秒杀券。
	CouponTypeSeckill = 2
	// CouponStatusDisabled 是停用。
	CouponStatusDisabled = 0
	// CouponStatusEnabled 是启用。
	CouponStatusEnabled = 1
)

// maxSourceLen 是 SyncRequest.Source 的最大长度。
const maxSourceLen = 32

// CouponPath 返回券详情的具体路径 /internal/v1/coupons/{id}。
func CouponPath(couponID int64) string {
	return "/internal/v1/coupons/" + strconv.FormatInt(couponID, 10)
}

// UserPath 返回用户详情的具体路径 /internal/v1/users/{id}。
func UserPath(userID int64) string {
	return "/internal/v1/users/" + strconv.FormatInt(userID, 10)
}

// invalid 构造 400 校验错误；服务端 handler 可直接返回，客户端在发请求前也用它快速失败。
func invalid(format string, args ...any) error {
	return apperr.InvalidRequest.WithMessage(fmt.Sprintf(format, args...))
}

func checkRequestID(id string) error {
	if !events.ValidRequestID(id) {
		return invalid("requestId 为空或含非法字符: %q", id)
	}
	return nil
}

func checkPositive(name string, v int64) error {
	if v <= 0 {
		return invalid("%s 必须为正数", name)
	}
	return nil
}

// DeductRequest 是 POST /internal/v1/stock/deduct 的请求体。
//
// 语义：为 (RequestID) 扣减 CouponID 的一件库存。同一 RequestID 重复调用幂等：
// 不会再扣第二次，而是返回第一次扣减所在的桶并置 AlreadyDeducted=true。
type DeductRequest struct {
	// RequestID 是秒杀请求 ID，库存账本主键。
	RequestID string `json:"requestId"`
	// CouponID 是被扣减的券。
	CouponID int64 `json:"couponId,string"`
	// UserID 决定起始桶（hash(userId) % N）。
	UserID int64 `json:"userId,string"`
}

// Validate 校验请求；失败返回 apperr.InvalidRequest。
func (r DeductRequest) Validate() error {
	if err := checkRequestID(r.RequestID); err != nil {
		return err
	}
	if err := checkPositive("couponId", r.CouponID); err != nil {
		return err
	}
	return checkPositive("userId", r.UserID)
}

// DeductResponse 是 stock/deduct 成功时的 data。
// 失败走信封：全部桶售罄 -> 409/OutOfStock，券不存在 -> 404/CouponNotFound，券停用 -> 409/CouponDisabled。
type DeductResponse struct {
	// CouponID 回显券 ID。
	CouponID int64 `json:"couponId,string"`
	// Bucket 是扣减所在的库存桶编号。
	Bucket int `json:"bucket"`
	// AlreadyDeducted 为 true 表示这是对同一 requestId 的重放，本次没有再扣。
	AlreadyDeducted bool `json:"alreadyDeducted"`
}

// ReleaseRequest 是 POST /internal/v1/stock/release 的请求体。
//
// 语义：归还 RequestID 之前扣掉的库存，幂等。建议 coupon 在没有任何扣减记录时也写入一条
// 已归还的墓碑，使迟到的 deduct 不会在 sweeper 已回滚之后又把库存扣走（见 doc.go）。
type ReleaseRequest struct {
	// RequestID 是要归还的扣减请求 ID。
	RequestID string `json:"requestId"`
	// CouponID 用于一致性校验，必须与扣减记录的券一致。
	CouponID int64 `json:"couponId,string"`
}

// Validate 校验请求；失败返回 apperr.InvalidRequest。
func (r ReleaseRequest) Validate() error {
	if err := checkRequestID(r.RequestID); err != nil {
		return err
	}
	return checkPositive("couponId", r.CouponID)
}

// ReleaseResponse 是 stock/release 成功时的 data。
// Released 与 AlreadyReleased 都为 false 表示该 requestId 没有任何扣减记录（无库存可还）。
type ReleaseResponse struct {
	// CouponID 回显券 ID。
	CouponID int64 `json:"couponId,string"`
	// Released 为 true 表示本次调用把库存还回了某个桶。
	Released bool `json:"released"`
	// AlreadyReleased 为 true 表示此前已经归还过，本次是重放。
	AlreadyReleased bool `json:"alreadyReleased"`
}

// 库存同步模式。缺省（空串）等价于 SyncModeMin。
const (
	// SyncModeMin 只降不升：生效库存 = min(当前可用库存, Available)。
	// 外部平台的库存读数可能是恒定值或过期值（例如 mock 平台恒为 500），按它「设置」会把已经卖出的库存
	// 重置回去造成超卖，所以自动同步一律用这个模式。
	SyncModeMin = "min"
	// SyncModeSet 强制设置为绝对值，可以抬高库存；只用于管理员明确要求的手工强制同步。
	SyncModeSet = "set"
)

// SyncRequest 是 POST /internal/v1/stock/sync 的请求体：按 Mode 调整券的可用库存并刷新 Redis。
type SyncRequest struct {
	// CouponID 是被同步的券。
	CouponID int64 `json:"couponId,string"`
	// Available 是外部来源给出的目标可用库存（非负绝对值，不是增量）。
	Available int64 `json:"available"`
	// Source 标识调用方，例如 "connector"、"admin"；仅用于审计日志。
	Source string `json:"source"`
	// Mode 是同步模式：SyncModeMin（缺省，只降不升）或 SyncModeSet（强制设置）。
	Mode string `json:"mode,omitempty"`
}

// EffectiveMode 返回生效的同步模式：空串按安全优先解释为 SyncModeMin。
func (r SyncRequest) EffectiveMode() string {
	if r.Mode == "" {
		return SyncModeMin
	}
	return r.Mode
}

// Validate 校验请求；失败返回 apperr.InvalidRequest。
func (r SyncRequest) Validate() error {
	if err := checkPositive("couponId", r.CouponID); err != nil {
		return err
	}
	if r.Available < 0 {
		return invalid("available 不能为负数")
	}
	if r.Mode != "" && r.Mode != SyncModeMin && r.Mode != SyncModeSet {
		return invalid("mode 只能是 %q 或 %q", SyncModeMin, SyncModeSet)
	}
	if r.Source == "" || len(r.Source) > maxSourceLen {
		return invalid("source 必须为 1 到 %d 个字符", maxSourceLen)
	}
	for i := 0; i < len(r.Source); i++ {
		c := r.Source[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return invalid("source 只能包含小写字母、数字、下划线和连字符")
		}
	}
	return nil
}

// SyncResponse 是 stock/sync 成功时的 data。Available 是**实际生效**的可用库存，
// min 模式下可能小于请求里的 Available。
type SyncResponse struct {
	// CouponID 回显券 ID。
	CouponID int64 `json:"couponId,string"`
	// Available 是同步后的可用库存。
	Available int64 `json:"available"`
}

// CouponInfo 是 GET /internal/v1/coupons/{id} 的 data：order 用来校验券并快照到订单。
// 金额使用 json.Number 保持两位小数的原样文本，不经过 float64。
type CouponInfo struct {
	// ID 是券 ID。
	ID int64 `json:"id,string"`
	// Name 是券名称，下单时快照到订单。
	Name string `json:"name"`
	// Type 是券类型，见 CouponType*。
	Type int `json:"type"`
	// Status 是券状态，见 CouponStatus*。
	Status int `json:"status"`
	// FaceValue 是面额。
	FaceValue json.Number `json:"faceValue"`
	// MinSpend 是使用门槛。
	MinSpend json.Number `json:"minSpend"`
	// ValidDays 是领取后的有效天数。
	ValidDays int `json:"validDays"`
	// PerUserLimit 是单用户领取上限。
	PerUserLimit int `json:"perUserLimit"`
	// SeckillStartAt 是秒杀开始时间，无时间窗为 null。
	SeckillStartAt *time.Time `json:"seckillStartAt"`
	// SeckillEndAt 是秒杀结束时间，无时间窗为 null。
	SeckillEndAt *time.Time `json:"seckillEndAt"`
}

// IsSeckill 判断是否秒杀券。
func (c CouponInfo) IsSeckill() bool { return c.Type == CouponTypeSeckill }

// Enabled 判断券是否启用。
func (c CouponInfo) Enabled() bool { return c.Status == CouponStatusEnabled }

// GrantRequest 是 POST /internal/v1/orders/grant 的请求体：直接给用户建一张订单（管理员发券）。
//
// 语义：不触碰库存，调用方（coupon）应先用同一个 RequestID 调 stock/deduct，失败时再 stock/release。
// 按 (UserID, RequestID) 幂等。
type GrantRequest struct {
	// RequestID 是本次发券的幂等键。
	RequestID string `json:"requestId"`
	// UserID 是受券用户。
	UserID int64 `json:"userId,string"`
	// CouponID 是被发的券。
	CouponID int64 `json:"couponId,string"`
}

// Validate 校验请求；失败返回 apperr.InvalidRequest。
func (r GrantRequest) Validate() error {
	if err := checkRequestID(r.RequestID); err != nil {
		return err
	}
	if err := checkPositive("userId", r.UserID); err != nil {
		return err
	}
	return checkPositive("couponId", r.CouponID)
}

// GrantResponse 是 orders/grant 成功时的 data。
// 用户已持有该券（部分唯一索引冲突）走信封 409/RepeatSeckill。
type GrantResponse struct {
	// OrderID 是生成的订单 ID。
	OrderID int64 `json:"orderId,string"`
	// AlreadyGranted 为 true 表示同一 requestId 此前已建过单，本次是重放。
	AlreadyGranted bool `json:"alreadyGranted"`
}

// UserInfo 是 GET /internal/v1/users/{id} 的 data：内部取展示名与角色，不含口令等敏感字段。
type UserInfo struct {
	// ID 是用户 ID。
	ID int64 `json:"id,string"`
	// Username 是用户名。
	Username string `json:"username"`
	// Role 是 "user" 或 "admin"。
	Role string `json:"role"`
	// Status 是用户状态。
	Status int `json:"status"`
}
