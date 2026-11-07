package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// releaseTimeout 是发券失败后归还库存的时间预算；它不随调用方 ctx 取消，避免客户端断开导致库存泄漏。
const releaseTimeout = 5 * time.Second

// GrantCommand 是管理员发券的入参。
type GrantCommand struct {
	// CouponID 是被发的券。
	CouponID int64
	// UserID 是受券用户。
	UserID int64
	// RequestID 是可选的幂等键：为空则生成一个新的；结果未知（503）时用同一个值重试是安全的。
	RequestID string
}

// GrantResult 是管理员发券的结果。
type GrantResult struct {
	// RequestID 是本次发券实际使用的幂等键。
	RequestID string
	// OrderID 是生成的订单 ID。
	OrderID int64
	// AlreadyGranted 为 true 表示同一 requestId 此前已建过单。
	AlreadyGranted bool
}

// Grant 给用户发一张券：先用同一个 requestId 调自己的 stock/deduct（进程内），再调 order 的 orders/grant，
// 建单被明确拒绝时归还库存。
//
// 失败语义（避免「订单已建而库存被归还」）：
//   - 扣减失败（售罄/券不存在/停用）：直接返回，没有副作用；
//   - 建单被明确拒绝（4xx，例如用户已持有该券）：幂等归还库存后返回原错误；
//   - 建单结果未知（网络错误、超时、5xx，ErrTransient 或 ctx 取消）：订单可能已建成，因此不归还，
//     返回 503 并在消息里带上 requestId，管理员用同一个 requestId 重试即可（deduct 与 grant 都按 requestId 幂等）。
//
// 成功后让 Redis 计数器同步减一，保持「Redis = DB 剩余」；该步失败只记日志（订单已建成，不能回滚）。
func (s *Service) Grant(ctx context.Context, cmd GrantCommand) (GrantResult, error) {
	if s.orders == nil {
		return GrantResult{}, apperr.SysError.WithCause(errors.New("order 客户端未配置"))
	}
	rid := cmd.RequestID
	if rid == "" {
		rid = fmt.Sprintf("grant-%d", s.ids.Next())
	}
	greq := internalapi.GrantRequest{RequestID: rid, UserID: cmd.UserID, CouponID: cmd.CouponID}
	if err := greq.Validate(); err != nil {
		return GrantResult{}, err
	}

	d, err := s.Deduct(ctx, internalapi.DeductRequest{RequestID: rid, CouponID: cmd.CouponID, UserID: cmd.UserID})
	if err != nil {
		return GrantResult{}, err
	}

	g, err := s.orders.GrantOrder(ctx, greq)
	if err != nil {
		return GrantResult{}, s.grantFailed(ctx, greq, err)
	}

	if !d.AlreadyDeducted {
		if terr := s.cache.TakeStock(ctx, cmd.CouponID); terr != nil {
			s.log.ErrorContext(ctx, "发券成功但 Redis 计数器未能同步减一（计数器偏大，不会超卖）",
				"coupon_id", cmd.CouponID, "request_id", rid, "err", terr)
		}
	}
	s.log.InfoContext(ctx, "管理员发券成功", "coupon_id", cmd.CouponID, "user_id", cmd.UserID,
		"request_id", rid, "order_id", g.OrderID, "already_granted", g.AlreadyGranted)
	return GrantResult{RequestID: rid, OrderID: g.OrderID, AlreadyGranted: g.AlreadyGranted}, nil
}

// grantFailed 处理建单失败：明确拒绝则归还库存并返回原错误，结果未知则保留扣减并提示重试。
func (s *Service) grantFailed(ctx context.Context, req internalapi.GrantRequest, cause error) error {
	if isAmbiguous(cause) {
		s.log.WarnContext(ctx, "建单结果未知，保留库存扣减，等待同一 requestId 重试",
			"coupon_id", req.CouponID, "user_id", req.UserID, "request_id", req.RequestID, "err", cause)
		return apperr.New(apperr.CodeSystemBusy, http.StatusServiceUnavailable,
			fmt.Sprintf("发券结果未知，请使用相同的 requestId=%s 重试", req.RequestID)).WithCause(cause)
	}

	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	if _, rerr := s.Release(rctx, internalapi.ReleaseRequest{RequestID: req.RequestID, CouponID: req.CouponID}); rerr != nil {
		s.log.ErrorContext(ctx, "建单被拒绝，且归还库存失败（库存暂时泄漏，可用同一 requestId 重试归还）",
			"coupon_id", req.CouponID, "request_id", req.RequestID, "grant_err", cause, "release_err", rerr)
		return fmt.Errorf("发券失败且归还库存失败: %w", errors.Join(cause, rerr))
	}
	s.log.InfoContext(ctx, "建单被拒绝，库存已归还",
		"coupon_id", req.CouponID, "user_id", req.UserID, "request_id", req.RequestID, "err", cause)
	return cause
}

// isAmbiguous 判断建单调用的结果是否未知：可重试的瞬时失败，或调用方 ctx 已取消/超时。
func isAmbiguous(err error) bool {
	return internalapi.IsTransient(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
