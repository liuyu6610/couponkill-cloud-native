// Package apperr 定义跨服务共享的业务错误：稳定的数字错误码 + 对应的 HTTP 状态码。
//
// 错误码沿用 Java 版的数值（前端、脚本、文档里都在用），HTTP 状态码随错误码走，
// 不再把所有业务失败都包成 HTTP 200。码值与映射见 design 文档 §4，改动需同步前端。
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// 业务错误码。数值保持与 Java 版 ErrorCodes 一致，新增码只能追加。
const (
	CodeOK = 200

	CodeCoolingDown              = 10001
	CodeOutOfStock               = 10002
	CodeRateLimited              = 10003
	CodeInvalidRequest           = 10004
	CodeNotPreheated             = 10005
	CodeNotStarted               = 10006
	CodeActivityEnded            = 10007
	CodeCouponNotFound           = 10008
	CodeCouponLimitExceeded      = 10009
	CodeRepeatSeckill            = 10010
	CodeOrderNotFound            = 10011
	CodeSeckillUseDedicatedAPI   = 10012
	CodeCouponServiceUnavailable = 10013
	CodeCouponDisabled           = 10014

	CodeAuthFail      = 20001
	CodeTokenInvalid  = 20002
	CodeUserNotFound  = 20003
	CodeUserExist     = 20004
	CodePasswordError = 20005
	CodeForbidden     = 20006

	CodeSysError   = 50000
	CodeSystemBusy = 50003
)

// Error 是带业务码与 HTTP 状态码的错误。零值不可用，请用 New 或下面的预定义错误。
type Error struct {
	Code       int
	HTTPStatus int
	Message    string
	cause      error
}

// New 创建一个业务错误。
func New(code, httpStatus int, message string) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: message}
}

// Error 实现 error 接口，包含底层原因，便于日志；对外响应只使用 Message。
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%d %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%d %s", e.Code, e.Message)
}

// Unwrap 返回底层原因，支持 errors.Is / errors.As 穿透。
func (e *Error) Unwrap() error { return e.cause }

// Is 按业务码判等：errors.Is(err, apperr.OutOfStock) 对任何 OutOfStock 副本都成立。
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// WithMessage 返回替换了对外文案的副本，不修改原（预定义）错误。
func (e *Error) WithMessage(message string) *Error {
	c := *e
	c.Message = message
	return &c
}

// WithCause 返回附带底层原因的副本；原因只进日志，不会出现在响应体里。
func (e *Error) WithCause(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// 预定义错误。使用时通常 return apperr.OutOfStock，需要定制文案用 .WithMessage。
var (
	CoolingDown              = New(CodeCoolingDown, http.StatusTooManyRequests, "操作过于频繁，请稍后再试")
	OutOfStock               = New(CodeOutOfStock, http.StatusConflict, "库存不足")
	RateLimited              = New(CodeRateLimited, http.StatusTooManyRequests, "请求过于频繁")
	InvalidRequest           = New(CodeInvalidRequest, http.StatusBadRequest, "请求参数错误")
	NotPreheated             = New(CodeNotPreheated, http.StatusConflict, "库存尚未预热")
	NotStarted               = New(CodeNotStarted, http.StatusConflict, "活动尚未开始")
	ActivityEnded            = New(CodeActivityEnded, http.StatusConflict, "活动已结束")
	CouponNotFound           = New(CodeCouponNotFound, http.StatusNotFound, "优惠券不存在")
	CouponLimitExceeded      = New(CodeCouponLimitExceeded, http.StatusConflict, "已超过领取上限")
	RepeatSeckill            = New(CodeRepeatSeckill, http.StatusConflict, "请勿重复秒杀")
	OrderNotFound            = New(CodeOrderNotFound, http.StatusNotFound, "订单不存在")
	SeckillUseDedicatedAPI   = New(CodeSeckillUseDedicatedAPI, http.StatusBadRequest, "秒杀券请使用秒杀接口")
	CouponServiceUnavailable = New(CodeCouponServiceUnavailable, http.StatusServiceUnavailable, "优惠券服务暂不可用")
	CouponDisabled           = New(CodeCouponDisabled, http.StatusConflict, "优惠券已停用")

	AuthFail      = New(CodeAuthFail, http.StatusUnauthorized, "未登录或登录已过期")
	TokenInvalid  = New(CodeTokenInvalid, http.StatusUnauthorized, "令牌无效")
	UserNotFound  = New(CodeUserNotFound, http.StatusNotFound, "用户不存在")
	UserExist     = New(CodeUserExist, http.StatusConflict, "用户名已存在")
	PasswordError = New(CodePasswordError, http.StatusUnauthorized, "用户名或密码错误")
	Forbidden     = New(CodeForbidden, http.StatusForbidden, "无权限访问")

	SysError   = New(CodeSysError, http.StatusInternalServerError, "系统错误")
	SystemBusy = New(CodeSystemBusy, http.StatusServiceUnavailable, "系统繁忙，请稍后再试")
)

// As 从错误链中取出 *Error。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Resolve 把任意 error 归一为对外的 *Error：非业务错误一律视为 SysError，
// 且不泄露内部信息（原错误保留为 cause，只用于日志）。nil 返回 nil。
func Resolve(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := As(err); ok {
		return e
	}
	return SysError.WithCause(err)
}

// CodeOf 返回错误对应的业务码；nil 为 CodeOK，非业务错误为 CodeSysError。
func CodeOf(err error) int {
	if err == nil {
		return CodeOK
	}
	return Resolve(err).Code
}

// StatusOf 返回错误对应的 HTTP 状态码；nil 为 200。
func StatusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return Resolve(err).HTTPStatus
}
