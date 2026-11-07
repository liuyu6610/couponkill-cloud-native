// Package httpx 是所有服务共用的 HTTP 层：gin 引擎工厂（recover → request-id → 访问日志 → 指标）、
// 统一 JSON 信封与错误映射、类型化的请求绑定、健康检查、鉴权中间件，以及出站 HTTP 客户端。
//
// 信封（design §4）：成功 {"code":200,"message":"ok","data":...}；
// 失败 {"code":10002,"message":"库存不足","data":null,"requestId":"..."}，HTTP 状态码随错误码走，
// 底层原因只写日志，不进响应体。
package httpx

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// Envelope 是所有响应的统一外壳。
type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	// RequestID 仅失败响应携带，便于用户报障时对上日志。
	RequestID string `json:"requestId,omitempty"`
}

// PageData 是分页响应的 data 结构。
type PageData struct {
	Items any   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// ctxKeyError 是 Fail 在 gin.Context 里记录最终业务错误的键，供访问日志读取。
const ctxKeyError = "httpx.error"

// OK 返回 200 与 data。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: apperr.CodeOK, Message: "ok", Data: data})
}

// Accepted 返回 202（异步受理，如秒杀入队）与 data。
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Envelope{Code: apperr.CodeOK, Message: "ok", Data: data})
}

// Page 返回分页数据；items 为 nil（含 nil 切片）时输出空数组 []，避免前端遇到 null。
func Page(c *gin.Context, items any, total int64, page, size int) {
	OK(c, PageData{Items: nonNilItems(items), Total: total, Page: page, Size: size})
}

func nonNilItems(items any) any {
	if items == nil {
		return []any{}
	}
	if v := reflect.ValueOf(items); v.Kind() == reflect.Slice && v.IsNil() {
		return reflect.MakeSlice(v.Type(), 0, 0).Interface()
	}
	return items
}

// Fail 把任意 error 归一为业务错误（apperr.Resolve）并写出对应 HTTP 状态与信封，随后中止后续 handler。
// 非业务错误一律表现为 50000 系统错误，响应体不含内部信息；完整原因写日志（5xx 为 Error 级，
// 4xx 为 Debug 级——访问日志已经记录了业务码，避免高峰期的拒绝洪水刷屏）。
func Fail(c *gin.Context, err error) {
	if err == nil {
		err = errors.New("httpx.Fail called with a nil error")
	}
	e := apperr.Resolve(err)
	log := logx.From(c.Request.Context())
	if e.HTTPStatus >= http.StatusInternalServerError {
		log.Error("request failed", "code", e.Code, "status", e.HTTPStatus, "err", err)
	} else {
		log.Debug("request rejected", "code", e.Code, "status", e.HTTPStatus, "err", err)
	}
	c.Set(ctxKeyError, e)
	c.AbortWithStatusJSON(e.HTTPStatus, Envelope{
		Code: e.Code, Message: e.Message, Data: nil, RequestID: logx.RequestID(c.Request.Context()),
	})
}
