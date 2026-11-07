package internalapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// decodeBody 解码测试服务器收到的 JSON 请求体。
func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}
