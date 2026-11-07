package service

import (
	"context"
	"fmt"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// Deduct 实现 POST /internal/v1/stock/deduct：按 requestId 幂等地扣减一件库存。
//
// 不变式（由 repo 在一个事务里保证）：同一 requestId 至多扣一次，重放返回第一次的桶并置 AlreadyDeducted；
// 已归还的 requestId 再扣返回 409/OutOfStock；全部桶售罄才算售罄；券不存在 404、停用 409。
// 本方法不碰 Redis：计数器的预扣与回补是 order 服务的职责。
func (s *Service) Deduct(ctx context.Context, req internalapi.DeductRequest) (internalapi.DeductResponse, error) {
	if err := req.Validate(); err != nil {
		return internalapi.DeductResponse{}, err
	}
	res, err := s.repo.Deduct(ctx, req)
	if err != nil {
		return internalapi.DeductResponse{}, fmt.Errorf("扣减库存 (requestId=%s coupon=%d): %w", req.RequestID, req.CouponID, err)
	}
	if !res.Already {
		s.log.DebugContext(ctx, "库存已扣减", "request_id", req.RequestID, "coupon_id", req.CouponID, "bucket", res.Bucket)
	}
	return internalapi.DeductResponse{CouponID: req.CouponID, Bucket: res.Bucket, AlreadyDeducted: res.Already}, nil
}

// Release 实现 POST /internal/v1/stock/release：按 requestId 幂等地归还库存。
//
// 只处理 DB 桶；若该 requestId 从未扣减，写入已归还的墓碑（Released 与 AlreadyReleased 都为 false），
// 使迟到的 deduct 被拒绝。Redis 计数器的回补由调用方（order）负责。
func (s *Service) Release(ctx context.Context, req internalapi.ReleaseRequest) (internalapi.ReleaseResponse, error) {
	if err := req.Validate(); err != nil {
		return internalapi.ReleaseResponse{}, err
	}
	res, err := s.repo.Release(ctx, req)
	if err != nil {
		return internalapi.ReleaseResponse{}, fmt.Errorf("归还库存 (requestId=%s coupon=%d): %w", req.RequestID, req.CouponID, err)
	}
	if res.Released {
		s.log.DebugContext(ctx, "库存已归还", "request_id", req.RequestID, "coupon_id", req.CouponID)
	}
	return internalapi.ReleaseResponse{CouponID: req.CouponID, Released: res.Released, AlreadyReleased: res.Already}, nil
}

// Sync 实现 POST /internal/v1/stock/sync：按 mode 在一个事务里调整可用库存，然后让 Redis 计数器等于 DB 剩余量。
//
// 缺省模式是 min（只降不升）：外部来源（连接器读到的平台库存）可能是恒定值或过期值，按它「设置」会把已卖出的
// 库存重置回去造成超卖。只有调用方显式传 mode=set（管理员手工强制同步）才允许抬高。
// 响应里的 Available 是实际生效值，min 模式下可能小于请求值。available 超过 MaxStock 返回 400。
func (s *Service) Sync(ctx context.Context, req internalapi.SyncRequest) (internalapi.SyncResponse, error) {
	if err := req.Validate(); err != nil {
		return internalapi.SyncResponse{}, err
	}
	if req.Available > MaxStock {
		return internalapi.SyncResponse{}, apperr.InvalidRequest.WithMessage(fmt.Sprintf("available 不能超过 %d", MaxStock))
	}
	mode := req.EffectiveMode()
	applied, err := s.repo.SetAvailable(ctx, req.CouponID, req.Available, s.cfg.StockBuckets, mode)
	if err != nil {
		return internalapi.SyncResponse{}, fmt.Errorf("同步库存 (coupon=%d): %w", req.CouponID, err)
	}
	s.log.InfoContext(ctx, "库存已同步", "coupon_id", req.CouponID, "mode", mode,
		"requested", req.Available, "applied", applied, "source", req.Source)
	if err := s.refresh(ctx, req.CouponID); err != nil {
		return internalapi.SyncResponse{}, err
	}
	return internalapi.SyncResponse{CouponID: req.CouponID, Available: applied}, nil
}
