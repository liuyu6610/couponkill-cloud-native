package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

var registerTagNames sync.Once

// useWireFieldNames 让校验错误里的字段名取 json/form/uri tag（前端看到的名字），而不是 Go 字段名。
func useWireFieldNames() {
	registerTagNames.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			for _, tag := range []string{"json", "form", "uri"} {
				if name, _, _ := strings.Cut(f.Tag.Get(tag), ","); name != "" && name != "-" {
					return name
				}
			}
			return f.Name
		})
	})
}

// BindJSON 把请求体按 JSON 绑定并校验为 T。失败时已写出 400 信封（message 含字段名与规则），
// 调用方只需 `if !ok { return }`。校验规则写在 `binding:"..."` tag 上。
func BindJSON[T any](c *gin.Context) (T, bool) {
	return bind[T](c, c.ShouldBindJSON)
}

// BindQuery 把查询串按 `form` tag 绑定并校验为 T，失败行为同 BindJSON。
func BindQuery[T any](c *gin.Context) (T, bool) {
	return bind[T](c, c.ShouldBindQuery)
}

// BindURI 把路径参数按 `uri` tag 绑定并校验为 T，失败行为同 BindJSON。
func BindURI[T any](c *gin.Context) (T, bool) {
	return bind[T](c, c.ShouldBindUri)
}

func bind[T any](c *gin.Context, fn func(any) error) (T, bool) {
	useWireFieldNames()
	var v T
	if err := fn(&v); err != nil {
		Fail(c, bindError(err))
		return v, false
	}
	return v, true
}

const maxProblems = 5

// bindError 把 gin/validator/encoding-json 的各种错误统一翻译为 400 信封。
func bindError(err error) error {
	invalid := func(msg string) error {
		return apperr.InvalidRequest.WithMessage("请求参数错误: " + msg).WithCause(err)
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		var probs []string
		for i, fe := range ve {
			if i == maxProblems {
				probs = append(probs, fmt.Sprintf("等 %d 项", len(ve)))
				break
			}
			probs = append(probs, fieldProblem(fe))
		}
		return invalid(strings.Join(probs, "; "))
	}
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		field := ute.Field
		if field == "" {
			field = "body"
		}
		return invalid(fmt.Sprintf("%s 类型错误（应为 %s）", field, ute.Type))
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return invalid("请求体过大")
	}
	var se *json.SyntaxError
	switch {
	case errors.As(err, &se), errors.Is(err, io.ErrUnexpectedEOF):
		return invalid("请求体不是合法的 JSON")
	case errors.Is(err, io.EOF):
		return invalid("请求体不能为空")
	}
	return invalid("参数格式错误")
}

func fieldProblem(fe validator.FieldError) string {
	name := fe.Field()
	isText := fe.Kind() == reflect.String || fe.Kind() == reflect.Slice || fe.Kind() == reflect.Array || fe.Kind() == reflect.Map
	bound := func(num, text string) string {
		if isText {
			return fmt.Sprintf("%s 长度%s %s", name, text, fe.Param())
		}
		return fmt.Sprintf("%s %s %s", name, num, fe.Param())
	}
	switch fe.Tag() {
	case "required":
		return name + " 为必填项"
	case "min", "gte":
		return bound("不能小于", "不能小于")
	case "max", "lte":
		return bound("不能大于", "不能大于")
	case "gt":
		return bound("必须大于", "必须大于")
	case "lt":
		return bound("必须小于", "必须小于")
	case "len":
		return fmt.Sprintf("%s 长度必须为 %s", name, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s 必须是 [%s] 之一", name, fe.Param())
	case "email":
		return name + " 不是合法的邮箱"
	}
	return fmt.Sprintf("%s 不满足 %s", name, fe.Tag())
}
