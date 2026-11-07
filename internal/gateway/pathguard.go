package gateway

import "strings"

// encodedAmbiguous 是转义路径里不允许出现的百分号编码（不区分大小写）：
// 编码后的 "."、"/"、"\"、";" 以及 "%" 本身（双重编码）。
// 网关解码一次后得到的路径与前缀白名单相符，但下游再解码/归一化一次就可能命中别的接口。
var encodedAmbiguous = []string{"%2e", "%2f", "%5c", "%3b", "%25"}

// ambiguousPath 报告路径是否含有会被下游再次归一化的歧义片段，对应 Java 版 InternalApiBlockFilter 的拦截意图：
//
//   - 转义路径里的 %2e %2f %5c %3b %25（含双重编码）；
//   - 解码路径里的 "//"、反斜杠、分号（路径参数）与点段（"." 和 ".."）；
//   - 任何控制字符（含 NUL 和 DEL），它们会让日志与下游解析出现分歧。
//
// escaped 是 URL.EscapedPath()，decoded 是 URL.Path。
func ambiguousPath(escaped, decoded string) bool {
	lower := strings.ToLower(escaped)
	for _, enc := range encodedAmbiguous {
		if strings.Contains(lower, enc) {
			return true
		}
	}
	if strings.Contains(decoded, "//") || strings.ContainsAny(decoded, `\;`) {
		return true
	}
	for _, r := range decoded {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	for _, seg := range strings.Split(decoded, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}
