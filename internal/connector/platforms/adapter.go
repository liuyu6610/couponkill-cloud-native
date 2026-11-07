// Package platforms 实现 connector 的电商平台适配层：PlatformAdapter 接口、带超时/重试/熔断的
// 出站 HTTP 封装（Caller）、各平台适配器（jd/pdd/tb/mock）以及按健康状态把关的注册表。
//
// 适配器只做“读”：查询商品与库存。库存如何推送给 coupon 服务由 service 包决定，
// 因此适配器永远不会出现在秒杀热路径里。
package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/domain"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/limiter"
)

// PlatformAdapter 是一个电商平台的只读适配器。实现必须并发安全。
type PlatformAdapter interface {
	// Platform 返回该适配器服务的平台。
	Platform() domain.Platform
	// Product 查询平台商品快照（标题、价格、是否在售）。
	Product(ctx context.Context, externalSkuID string) (domain.ProductSnapshot, error)
	// Stock 查询平台库存快照；平台只给“有货/无货”时 StockQty 为 nil。
	Stock(ctx context.Context, externalSkuID string) (domain.StockSnapshot, error)
	// Health 报告当前健康状态。必须便宜（不得发起外部调用）：注册表会按 TTL 惰性复查。
	Health(ctx context.Context) domain.Health
}

// cst 是东八区。中国没有夏令时，用固定偏移而不是 LoadLocation，
// 因为 distroless 运行镜像里没有 tzdata。
var cst = time.FixedZone("CST", 8*3600)

// CST 返回东八区，供平台签名时间戳与比价展示使用。
func CST() *time.Location { return cst }

// priceRE 只接受非负的十进制小数文本（拒绝科学计数法、符号、NaN/Inf）。
var priceRE = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,6})?$`)

// price 把平台给出的价格文本校验后转成 JSON 数字；空文本返回 nil。
func price(text string) (*json.Number, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if !priceRE.MatchString(text) {
		return nil, fmt.Errorf("invalid price %q: %w", text, ErrBadResponse)
	}
	whole, frac, _ := strings.Cut(text, ".")
	frac += "000" // 补足到至少 3 位，便于按第 3 位四舍五入
	cents, err := strconv.ParseInt(whole+frac[:2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid price %q: %w", text, ErrBadResponse)
	}
	if frac[2] >= '5' {
		cents++
	}
	return centsToPrice(cents), nil
}

// centsToPrice 把以“分”为单位的整数转成元的 JSON 数字（如 9900 -> 99.00）。
func centsToPrice(cents int64) *json.Number {
	n := json.Number(fmt.Sprintf("%d.%02d", cents/100, cents%100))
	return &n
}

// credentialedHealth 是需要凭证的 HTTP 适配器共用的健康判定（不发起外部调用）：
// 缺凭证 DISABLED；熔断器打开 DOWN；否则 UP。
func credentialedHealth(p domain.Platform, configured bool, missing string, c *Caller) domain.Health {
	switch {
	case !configured:
		return domain.Health{Platform: p, Status: domain.StatusDisabled, Message: missing}
	case c.BreakerState() == limiter.Open:
		return domain.Health{Platform: p, Status: domain.StatusDown, Message: "近期调用持续失败，熔断器已打开，稍后自动半开探测"}
	}
	return domain.Health{Platform: p, Status: domain.StatusUp, Message: "credentials configured（未主动探活，可用 /probe 验证）"}
}

// ptr 返回值的指针。
func ptr[T any](v T) *T { return &v }
