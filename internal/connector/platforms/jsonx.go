package platforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// decodeJSON 把平台响应解析成通用树（数字保留为 json.Number，避免大整数精度丢失）。
// 空响应按空对象处理，非法 JSON 返回 ErrBadResponse。
func decodeJSON(raw []byte) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode platform response: %w", ErrBadResponse)
	}
	return v, nil
}

// at 沿 path 逐级取对象字段；任何一级缺失或类型不符返回 nil。
func at(v any, path ...string) any {
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}
	return v
}

// asString 取字符串；数字与布尔按文本形式返回；nil 返回 false。
func asString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		return strconv.FormatBool(x), true
	}
	return "", false
}

// asInt64 取整数；接受 JSON 数字与数字文本，小数截断；无法解析返回 false。
func asInt64(v any) (int64, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = strings.TrimSpace(x)
	default:
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
	}
	return 0, false
}

// firstOf 在候选路径里找第一个“非空数组”，返回其首元素；都没有返回 nil。
func firstOf(root any, candidates ...[]string) map[string]any {
	for _, path := range candidates {
		if arr, ok := at(root, path...).([]any); ok && len(arr) > 0 {
			if m, ok := arr[0].(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

// errorResponse 检查平台的 error_response 包装，存在则返回 RejectedError。
// codeKey/msgKey 因平台而异；返回的描述会被截断，且只含平台给出的错误码与文案。
func errorResponse(platform string, root any, codeKey, msgKey string) error {
	er := at(root, "error_response")
	m, ok := er.(map[string]any)
	if !ok {
		return nil
	}
	code, _ := asString(m[codeKey])
	msg, _ := asString(m[msgKey])
	if sub, ok := asString(m["sub_msg"]); ok && sub != "" {
		msg += " / " + sub
	}
	if len(msg) > errSnippetBytes {
		msg = msg[:errSnippetBytes]
	}
	return &RejectedError{Platform: platform, Code: code, Message: msg}
}
