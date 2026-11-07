package platforms

import (
	"errors"
	"fmt"
)

// 适配器对外暴露的错误种类。上层用 errors.Is / errors.As 区分，不解析文案。
var (
	// ErrDisabled 表示平台缺少凭证，被禁用；不会发出任何外部请求。
	ErrDisabled = errors.New("platforms: platform disabled (credentials missing)")
	// ErrCircuitOpen 表示该平台的熔断器处于打开状态，调用被直接拒绝。
	ErrCircuitOpen = errors.New("platforms: circuit breaker open")
	// ErrBadResponse 表示平台返回了无法解析或超出大小上限的响应。
	ErrBadResponse = errors.New("platforms: bad platform response")
	// ErrNotFound 表示平台没有返回请求的 SKU。
	ErrNotFound = errors.New("platforms: sku not found on platform")
	// ErrUnknownPlatform 表示请求的平台没有注册适配器。
	ErrUnknownPlatform = errors.New("platforms: unknown platform")
)

// UpstreamError 描述一次到平台的 HTTP 调用失败：Status 为 0 表示传输层错误（超时、连接被拒等）。
// 传输错误与 5xx/429 是可重试的，并计入熔断器；其余 4xx 是平台明确的拒绝，不重试也不计入。
type UpstreamError struct {
	// Platform 是出错的平台名，仅用于日志与文案。
	Platform string
	// Status 是 HTTP 状态码，传输错误为 0。
	Status int
	// Err 是底层原因（传输错误或带响应片段的描述）。
	Err error
}

// Error 实现 error。
func (e *UpstreamError) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("%s upstream transport error: %v", e.Platform, e.Err)
	}
	return fmt.Sprintf("%s upstream http %d: %v", e.Platform, e.Status, e.Err)
}

// Unwrap 返回底层原因。
func (e *UpstreamError) Unwrap() error { return e.Err }

// Retryable 报告该失败是否值得重试（并计入熔断器）：传输错误、429 与 5xx。
func (e *UpstreamError) Retryable() bool {
	return e.Status == 0 || e.Status == 429 || e.Status >= 500
}

// RejectedError 表示平台在 HTTP 200 的响应里以业务错误拒绝了请求（如京东的 error_response）。
// 这类错误通常是凭证过期或参数问题，重试无益，也不说明平台宕机，所以不计入熔断器。
type RejectedError struct {
	// Platform 是平台名。
	Platform string
	// Code 是平台给出的错误码（可能为空）。
	Code string
	// Message 是平台给出的错误描述，已截断，不含凭证。
	Message string
}

// Error 实现 error。
func (e *RejectedError) Error() string {
	return fmt.Sprintf("%s rejected request: code=%s msg=%s", e.Platform, e.Code, e.Message)
}
