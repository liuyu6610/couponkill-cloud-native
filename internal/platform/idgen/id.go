package idgen

import (
	"bytes"
	"fmt"
	"strconv"
)

// ID 是对外 JSON 里的 64 位 ID：输出一律为字符串（避免 JS 精度丢失，design D8），
// 输入同时接受字符串与整数。
type ID int64

// ParseID 严格解析十进制 ID 字符串（不接受空白、符号以外的字符或溢出）。
func ParseID(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("idgen: invalid id %q", s)
	}
	return n, nil
}

// String 返回十进制字符串。
func (i ID) String() string { return strconv.FormatInt(int64(i), 10) }

// MarshalJSON 输出带引号的十进制字符串。
func (i ID) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, i.String()), nil
}

// UnmarshalJSON 接受 "123" 或 123；null 保持零值。
func (i *ID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	raw := b
	if len(b) > 0 && b[0] == '"' {
		s, err := strconv.Unquote(string(b))
		if err != nil {
			return fmt.Errorf("idgen: invalid id %s", b)
		}
		raw = []byte(s)
	}
	n, err := ParseID(string(raw))
	if err != nil {
		return err
	}
	*i = ID(n)
	return nil
}
