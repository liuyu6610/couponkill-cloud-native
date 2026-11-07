package platforms

import (
	"crypto/md5" //nolint:gosec // 京东/淘宝/拼多多开放平台规定的签名算法就是 MD5，不是我们的安全选择
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// kv 是有序的键值对。京东的 360buy_param_json 要求按插入顺序序列化业务参数。
type kv struct{ k, v string }

// md5Sign 实现三家开放平台共用的 MD5 签名：把除 sign 外的参数按键名升序拼成 k1v1k2v2…，
// 前后各加一次 secret，取 MD5 的大写十六进制。参数值按 UTF-8 参与计算。
func md5Sign(secret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(secret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(secret)
	sum := md5.Sum([]byte(sb.String())) //nolint:gosec // 见包导入处说明
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// formValues 把参数转成 application/x-www-form-urlencoded 表单。
func formValues(params map[string]string) url.Values {
	form := make(url.Values, len(params))
	for k, v := range params {
		form.Set(k, v)
	}
	return form
}

// jsonObject 按给定顺序把键值对序列化为 JSON 对象（值一律是字符串），
// 转义规则与 Java 版一致：只转义反斜杠与双引号，保证签名内容逐字节一致。
func jsonObject(pairs []kv) string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`"` + jsonEscape(p.k) + `":"` + jsonEscape(p.v) + `"`)
	}
	sb.WriteByte('}')
	return sb.String()
}

func jsonEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
