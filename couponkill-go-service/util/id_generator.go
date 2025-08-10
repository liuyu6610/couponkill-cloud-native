package util

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

// GenerateOrderID 生成订单ID，前缀"GO-"区分Go服务创建的订单
func GenerateOrderID() string {
	timestamp := time.Now().UnixNano() / 1000000 // 毫秒时间戳
	b := make([]byte, 8)
	rand.Read(b)
	randomStr := base64.URLEncoding.EncodeToString(b)[:8]
	return "GO-" + time.Unix(0, timestamp*1000000).Format("20060102150405") + "-" + randomStr
}
