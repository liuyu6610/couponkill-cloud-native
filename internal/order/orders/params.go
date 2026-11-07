package orders

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/httpx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// IDParam 读取请求里名为 name 的 ID 参数：优先取 JSON 体里的同名字段（值可为字符串或数字），
// 没有 JSON 体时回落到同名查询参数（兼容旧前端的 ?couponId=）。
// 缺失、非法或非正数时已写好 400 信封并返回 ok=false，调用方直接 return。
func IDParam(c *gin.Context, name string) (int64, bool) {
	if c.Request.ContentLength != 0 && strings.Contains(c.GetHeader("Content-Type"), "json") {
		body, ok := httpx.BindJSON[map[string]idgen.ID](c)
		if !ok {
			return 0, false
		}
		if v := int64(body[name]); v > 0 {
			return v, true
		}
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: "+name+" 为必填项且必须为正数"))
		return 0, false
	}
	raw := c.Query(name)
	if raw == "" {
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: "+name+" 为必填项"))
		return 0, false
	}
	v, err := idgen.ParseID(raw)
	if err != nil || v <= 0 {
		httpx.Fail(c, apperr.InvalidRequest.WithMessage("请求参数错误: "+name+" 不是合法的 ID"))
		return 0, false
	}
	return v, true
}
