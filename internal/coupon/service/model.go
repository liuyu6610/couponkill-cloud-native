// Package service 实现 coupon 服务的业务规则：券的创建与状态机、分桶库存的扣减/归还/同步、
// 管理员发券，以及 Redis 库存计数器与元数据缓存的刷新。
//
// 本包只依赖两个自己定义的小接口：Repo（PostgreSQL，权威数据）与 Cache（Redis，派生数据），
// 因此全部业务规则都能用假实现单测；库存的并发/幂等不变式由 repo 包在真实 PostgreSQL 上证明。
//
// 一致性模型：PostgreSQL 的 coupon_stock 各桶之和是库存的唯一权威；
// Redis 里的 ck:stock:{id} 只是它的派生值，DB 变更提交之后才刷新，刷新失败会如实报错
// （管理员可用预热接口重试），不会静默漂移。
package service

import (
	"context"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
)

// 券类型与状态，与 internalapi、前端 types/api.ts 一致。
const (
	// TypeNormal 是常驻券。
	TypeNormal = internalapi.CouponTypeNormal
	// TypeSeckill 是秒杀券。
	TypeSeckill = internalapi.CouponTypeSeckill
	// StatusDisabled 是停用。
	StatusDisabled = internalapi.CouponStatusDisabled
	// StatusEnabled 是启用。
	StatusEnabled = internalapi.CouponStatusEnabled
)

// Coupon 是券的读模型：模板字段加上由库存桶汇总出的剩余量。
type Coupon struct {
	// ID 是雪花 ID。
	ID int64
	// Name 是券名称。
	Name string
	// Description 是券描述，可为空串。
	Description string
	// Type 是券类型（TypeNormal / TypeSeckill）。
	Type int
	// Status 是券状态（StatusDisabled / StatusEnabled）。
	Status int
	// FaceValue 是面额，两位小数的十进制文本（如 "50.00"），不经过浮点数。
	FaceValue string
	// MinSpend 是使用门槛，格式同 FaceValue。
	MinSpend string
	// ValidDays 是领取后的有效天数。
	ValidDays int
	// PerUserLimit 是单用户领取上限。
	PerUserLimit int
	// TotalStock 是发行总量（秒杀券即秒杀总库存）。
	TotalStock int64
	// Remaining 是剩余库存：coupon_stock 各桶之和。
	Remaining int64
	// SeckillStartAt 是秒杀开始时间，无时间窗为 nil。
	SeckillStartAt *time.Time
	// SeckillEndAt 是秒杀结束时间，无时间窗为 nil。
	SeckillEndAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// IsSeckill 判断是否秒杀券。
func (c Coupon) IsSeckill() bool { return c.Type == TypeSeckill }

// Enabled 判断券是否启用。
func (c Coupon) Enabled() bool { return c.Status == StatusEnabled }

// NewCoupon 是经过校验、可直接落库的新券（金额已规范化，库存已决定）。
type NewCoupon struct {
	ID             int64
	Name           string
	Description    string
	Type           int
	FaceValue      string
	MinSpend       string
	ValidDays      int
	PerUserLimit   int
	TotalStock     int64
	SeckillStartAt *time.Time
	SeckillEndAt   *time.Time
}

// StockState 是刷新 Redis 所需的最小状态：类型、状态与 DB 剩余库存。
type StockState struct {
	Type      int
	Status    int
	Remaining int64
}

// ShouldHaveCounter 报告该券是否应当存在 ck:stock 计数器：仅「启用中的秒杀券」。
func (s StockState) ShouldHaveCounter() bool {
	return s.Type == TypeSeckill && s.Status == StatusEnabled
}

// DeductResult 是一次扣减的结果。
type DeductResult struct {
	// Bucket 是扣减所在的桶。
	Bucket int
	// Already 为 true 表示这是对同一 requestId 的重放，本次没有再扣。
	Already bool
}

// ReleaseResult 是一次归还的结果。Released 与 Already 都为 false 表示该 requestId 此前
// 没有扣减记录（已写入墓碑）。
type ReleaseResult struct {
	// Released 为 true 表示本次把库存还回了某个桶。
	Released bool
	// Already 为 true 表示此前已经归还过。
	Already bool
}

// Repo 是 PostgreSQL 存储抽象。业务失败直接返回 apperr 错误：券不存在为 apperr.CouponNotFound，
// 全桶售罄为 apperr.OutOfStock，券停用为 apperr.CouponDisabled；其余为需要上层包装的系统错误。
type Repo interface {
	// Available 返回启用且未结束（无时间窗，或结束时间晚于 now）的券，按创建时间倒序，最多 limit 条。
	Available(ctx context.Context, now time.Time, limit int) ([]Coupon, error)
	// List 分页返回券（创建时间倒序）与总数；enabledOnly 为 true 时只含启用券。
	List(ctx context.Context, offset, limit int, enabledOnly bool) ([]Coupon, int64, error)
	// Get 返回一张券；不存在返回 apperr.CouponNotFound。
	Get(ctx context.Context, id int64) (Coupon, error)
	// Create 在一个事务里写入券与各库存桶（buckets[i] 是第 i 桶的初始库存），返回落库后的券；ID 已存在返回 apperr 409。
	Create(ctx context.Context, c NewCoupon, buckets []int64) (Coupon, error)
	// UpdateWindow 设置秒杀时间窗；券不存在返回 apperr.CouponNotFound。
	UpdateWindow(ctx context.Context, id int64, start, end time.Time) error
	// UpdateStatus 设置券状态；券不存在返回 apperr.CouponNotFound。
	UpdateStatus(ctx context.Context, id int64, status int) error
	// Delete 删除券与其库存桶（库存流水保留作审计），返回删除的券数（0 或 1）。
	Delete(ctx context.Context, id int64) (int, error)
	// Deduct 在一个事务里完成「幂等占位 + 桶扣减 + 流水」，见 repo 包文档。
	Deduct(ctx context.Context, req internalapi.DeductRequest) (DeductResult, error)
	// Release 在一个事务里完成「幂等归还 + 桶回补 + 流水」，无扣减记录时写入墓碑。
	Release(ctx context.Context, req internalapi.ReleaseRequest) (ReleaseResult, error)
	// SetAvailable 在一个事务里按 mode 调整可用库存并返回实际生效值：SyncModeSet 设置为绝对值（可抬高，
	// 并保证发行总量不小于它）；SyncModeMin 只降不升，生效值 = min(当前剩余, available)。
	// 券还没有任何库存桶时没有当前值可保护，两种模式都按首次设置处理。
	SetAvailable(ctx context.Context, id int64, available int64, defaultBuckets int, mode string) (int64, error)
	// StockState 读取刷新 Redis 所需的状态；券不存在返回 apperr.CouponNotFound。
	StockState(ctx context.Context, id int64) (StockState, error)
	// EnabledSeckillIDs 返回所有启用中的秒杀券 ID。
	EnabledSeckillIDs(ctx context.Context) ([]int64, error)
}

// Cache 是 Redis 抽象：库存计数器 ck:stock:{id} 与元数据缓存 ck:cpn:meta:{id}。
type Cache interface {
	// PutStock 无条件把计数器设为 remaining。
	PutStock(ctx context.Context, couponID, remaining int64) error
	// PutStockIfAbsent 仅在计数器不存在时写入，返回是否写入。
	PutStockIfAbsent(ctx context.Context, couponID, remaining int64) (bool, error)
	// DropStock 删除计数器（不存在不是错误）。
	DropStock(ctx context.Context, couponID int64) error
	// TakeStock 在计数器存在且大于 0 时减一（管理员发券占用库存后保持 Redis 与 DB 一致），否则什么也不做。
	TakeStock(ctx context.Context, couponID int64) error
	// DropMeta 删除券元数据缓存（不存在不是错误）。
	DropMeta(ctx context.Context, couponID int64) error
}

// IDSource 生成雪花 ID；*idgen.Generator 满足它。
type IDSource interface {
	// Next 返回下一个 ID。
	Next() int64
}
