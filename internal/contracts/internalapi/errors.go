package internalapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// ErrTransient 标记「可以稍后重试」的失败：网络错误、超时、5xx。
//
// 它包装 apperr.SystemBusy，所以 errors.Is(err, ErrTransient) 与 errors.Is(err, apperr.SystemBusy)
// 都成立。客户端返回的 transient 错误的最外层仍是 *apperr.Error（网络错误为 SystemBusy，
// 无信封的 5xx 为各客户端的「服务不可用」错误，有信封的 5xx 保留下游的原码与状态码），
// 因此直接交给 httpx 渲染也是正确的 503。
//
// 消费者的用法：业务终态错误（例如 409/OutOfStock）不是 transient，直接按终态处理；
// ErrTransient 则返回错误让 Kafka 重试，同一 requestId 的重试是安全的。
var ErrTransient = fmt.Errorf("internalapi: transient failure: %w", apperr.SystemBusy)

// IsTransient 判断错误是否可以稍后重试。
func IsTransient(err error) bool { return errors.Is(err, ErrTransient) }

// knownErrors 是 apperr 的全部预定义错误，用来在下游响应的 HTTP 状态码与信封不一致时，
// 按业务码取规范的状态码。
var knownErrors = []*apperr.Error{
	apperr.CoolingDown, apperr.OutOfStock, apperr.RateLimited, apperr.InvalidRequest,
	apperr.NotPreheated, apperr.NotStarted, apperr.ActivityEnded, apperr.CouponNotFound,
	apperr.CouponLimitExceeded, apperr.RepeatSeckill, apperr.OrderNotFound,
	apperr.SeckillUseDedicatedAPI, apperr.CouponServiceUnavailable, apperr.CouponDisabled,
	apperr.AuthFail, apperr.TokenInvalid, apperr.UserNotFound, apperr.UserExist,
	apperr.PasswordError, apperr.Forbidden, apperr.SysError, apperr.SystemBusy,
}

// canonicalStatus 返回业务码的规范 HTTP 状态码，未知码返回 0。
func canonicalStatus(code int) int {
	for _, e := range knownErrors {
		if e.Code == code {
			return e.HTTPStatus
		}
	}
	return 0
}

// isUserAuthCode 判断业务码是否属于「终端用户鉴权」类。
// 内部调用收到这类码说明服务间令牌/网关配置有误，绝不能原样透传给终端用户
// （否则前端会把用户登出），因此统一降级为 SysError。
func isUserAuthCode(code int) bool {
	switch code {
	case apperr.CodeAuthFail, apperr.CodeTokenInvalid, apperr.CodePasswordError, apperr.CodeForbidden:
		return true
	}
	return false
}

// remoteError 把下游的失败响应（信封或原始状态码）转成 *apperr.Error。
// code 为 0 表示响应体不是合法信封。unavailable 是无信封 5xx 使用的错误。
func remoteError(status, code int, message string, unavailable *apperr.Error, cause error) error {
	switch {
	case code != 0 && isUserAuthCode(code):
		return apperr.SysError.WithCause(fmt.Errorf("internal call rejected (HTTP %d, code %d %q): %w", status, code, message, cause))
	case code != 0:
		httpStatus := status
		if status >= 200 && status < 300 {
			httpStatus = canonicalStatus(code)
			if httpStatus == 0 {
				httpStatus = http.StatusInternalServerError
			}
		}
		app := apperr.New(code, httpStatus, message).WithCause(cause)
		if httpStatus >= 500 {
			return markTransient(app)
		}
		return app
	case status >= 500:
		return markTransient(unavailable.WithCause(cause))
	default:
		return apperr.SysError.WithCause(fmt.Errorf("internal call failed with unrecognized HTTP %d: %w", status, cause))
	}
}

// markTransient 在错误链里加入 ErrTransient，同时保持最外层是原来的 *apperr.Error。
func markTransient(app *apperr.Error) error {
	cause := app.Unwrap()
	if cause == nil {
		return app.WithCause(ErrTransient)
	}
	return app.WithCause(fmt.Errorf("%w: %w", ErrTransient, cause))
}
