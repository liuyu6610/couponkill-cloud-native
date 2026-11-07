// Package domain 定义 connector 服务的领域类型：平台枚举、绑定与价格映射、平台快照、
// 同步结果，以及本服务专有的业务错误。它不依赖 PG/Redis/HTTP，是 platforms、repo、service
// 之间共享的词汇表。JSON 字段名与前端 types/api.ts 对齐（ID 一律以字符串输出）。
package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/idgen"
)

// Platform 是电商平台标识，取值与 cnx 表里 platform 列的 CHECK 约束一致（大写）。
type Platform string

// 已支持的平台。
const (
	// Mock 是本地联调用的模拟平台，永远可用。
	Mock Platform = "MOCK"
	// JD 是京东开放平台（JOS）。
	JD Platform = "JD"
	// TB 是淘宝开放平台（TOP）。
	TB Platform = "TB"
	// PDD 是拼多多开放平台。
	PDD Platform = "PDD"
)

// AllPlatforms 按展示顺序列出全部平台（与 Java 版 EnumMap 顺序一致）。
var AllPlatforms = []Platform{Mock, JD, TB, PDD}

// ParsePlatform 去空白并转大写后解析平台；未知平台返回 400。
func ParsePlatform(s string) (Platform, error) {
	p := Platform(strings.ToUpper(strings.TrimSpace(s)))
	if p == "" {
		return "", apperr.InvalidRequest.WithMessage("platform 不能为空")
	}
	for _, known := range AllPlatforms {
		if p == known {
			return p, nil
		}
	}
	return "", apperr.InvalidRequest.WithMessage("未知平台: " + strings.TrimSpace(s))
}

// UnmarshalJSON 让请求体里的 platform 字段大小写不敏感（去空白并转大写）。
// 这里不拒绝未知平台：由 service 层用 ParsePlatform 校验，这样调用方得到的是带具体原因的 400，
// 而不是 JSON 绑定层笼统的“参数格式错误”。
func (p *Platform) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("platform 必须是字符串: %w", err)
	}
	*p = Platform(strings.ToUpper(strings.TrimSpace(s)))
	return nil
}

// Key 返回 "平台|SKU" 形式的去重键。
func (p Platform) Key(sku string) string { return string(p) + "|" + sku }

// 同步状态，取值与 last_sync_status 列的 CHECK 约束一致。
const (
	// SyncNew 表示绑定刚创建，尚未同步过。
	SyncNew = "NEW"
	// SyncSkip 表示平台没有给出可用的精确库存，本次未写入。
	SyncSkip = "SKIP"
	// SyncSuccess 表示库存已推送给 coupon 服务。
	SyncSuccess = "SUCCESS"
	// SyncFail 表示平台调用或推送失败。
	SyncFail = "FAIL"
)

// Binding 是平台 SKU 与本地券的绑定，同时承载库存同步进度。
type Binding struct {
	ID             idgen.ID   `json:"id"`
	Platform       Platform   `json:"platform"`
	ExternalSkuID  string     `json:"externalSkuId"`
	CouponID       idgen.ID   `json:"couponId"`
	SyncEnabled    bool       `json:"syncEnabled"`
	LastStock      *int64     `json:"lastStock"`
	LastSyncAt     *time.Time `json:"lastSyncAt"`
	LastSyncStatus *string    `json:"lastSyncStatus"`
	LastError      *string    `json:"lastError"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// BindingCommand 是创建/更新绑定的请求体。couponId 同时接受数字与字符串。
type BindingCommand struct {
	Platform      Platform `json:"platform"`
	ExternalSkuID string   `json:"externalSkuId"`
	CouponID      idgen.ID `json:"couponId"`
	// SyncEnabled 缺省为 true。
	SyncEnabled *bool `json:"syncEnabled"`
}

// SyncOutcome 是一次同步尝试写回绑定的结果。
type SyncOutcome struct {
	// Status 是 SyncSuccess / SyncSkip / SyncFail。
	Status string
	// LastStock 非 nil 时覆盖 last_stock；nil 保持原值（失败与跳过不得抹掉上次成功值）。
	LastStock *int64
	// Error 是 last_error；nil 清空。
	Error *string
	// At 是写入 last_sync_at 的时间（调用方时钟，UTC）。
	At time.Time
}

// PriceMap 是同品比价的手工映射（多平台参考价；不参与库存同步）。
type PriceMap struct {
	ID            idgen.ID     `json:"id"`
	CouponID      idgen.ID     `json:"couponId"`
	Platform      Platform     `json:"platform"`
	ExternalSkuID string       `json:"externalSkuId"`
	Title         *string      `json:"title"`
	ManualPrice   *json.Number `json:"manualPrice"`
	Currency      string       `json:"currency"`
	Enabled       bool         `json:"enabled"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

// PriceMapCommand 是创建/更新价格映射的请求体。
type PriceMapCommand struct {
	CouponID      idgen.ID     `json:"couponId"`
	Platform      Platform     `json:"platform"`
	ExternalSkuID string       `json:"externalSkuId"`
	Title         *string      `json:"title"`
	ManualPrice   *json.Number `json:"manualPrice"`
	// Currency 缺省为 CNY。
	Currency string `json:"currency"`
	// Enabled 缺省为 true。
	Enabled *bool `json:"enabled"`
}

// ProductSnapshot 是平台商品快照（只读）。
type ProductSnapshot struct {
	Platform      Platform     `json:"platform"`
	ExternalSkuID string       `json:"externalSkuId"`
	Title         string       `json:"title"`
	Price         *json.Number `json:"price"`
	OnSale        *bool        `json:"onSale"`
	RawStatus     string       `json:"rawStatus"`
}

// StockSnapshot 是平台库存快照。StockQty 为 nil 表示平台只给了“有货/无货”而没有精确数量。
type StockSnapshot struct {
	Platform       Platform `json:"platform"`
	ExternalSkuID  string   `json:"externalSkuId"`
	StockQty       *int64   `json:"stockQty"`
	StockAvailable *bool    `json:"stockAvailable"`
	StockStateDesc string   `json:"stockStateDesc"`
	Area           string   `json:"area"`
}

// 适配器健康状态。
const (
	// StatusUp 表示凭证齐全且熔断器关闭，可以调用。
	StatusUp = "UP"
	// StatusDown 表示凭证齐全但熔断器打开（近期连续失败）。
	StatusDown = "DOWN"
	// StatusDisabled 表示缺少凭证，平台被禁用。
	StatusDisabled = "DISABLED"
)

// Health 是单个平台的健康状态。
type Health struct {
	Platform Platform `json:"platform"`
	Status   string   `json:"status"`
	Message  string   `json:"message"`
}

// PriceCompareItem 是同品比价的一行。
type PriceCompareItem struct {
	Platform      Platform     `json:"platform"`
	ExternalSkuID string       `json:"externalSkuId"`
	Title         *string      `json:"title"`
	Price         *json.Number `json:"price"`
	Currency      string       `json:"currency"`
	// FetchedAt 是取价时间（RFC3339，与前端 Go 契约一致；Java 版是东八区的 "yyyy-MM-dd HH:mm:ss"）。
	FetchedAt time.Time `json:"fetchedAt"`
	// Source 是 PROBE 或 MANUAL。
	Source string `json:"source"`
	// Confidence 是 HIGH / MEDIUM / LOW。
	Confidence string  `json:"confidence"`
	Message    *string `json:"message"`
}

// PriceCompareResult 是同品比价结果。
type PriceCompareResult struct {
	CouponID  idgen.ID           `json:"couponId"`
	BindingID *idgen.ID          `json:"bindingId"`
	Items     []PriceCompareItem `json:"items"`
}

// SyncBatchResult 是一批同步的统计。
type SyncBatchResult struct {
	SyncedOK int `json:"syncedOk"`
	Failed   int `json:"failed"`
	// Skipped 包含平台未给精确库存的 SKIP 与因预算/丢锁顺延的绑定。
	Skipped int `json:"skipped"`
	// Force 回显请求参数（见 README：coupon 内部接口是绝对值设置，force 不改变推送语义）。
	Force bool `json:"force"`
}

// ErrSyncBusy 表示库存同步锁正被定时任务、其它实例或另一次手工同步持有（HTTP 409）。
// 业务码沿用 SystemBusy（50003）以免新增码值，调用方应稍后重试。
var ErrSyncBusy = apperr.New(apperr.CodeSystemBusy, http.StatusConflict, "库存同步进行中（定时任务或其它实例持有同步锁），请稍后重试")

// ErrSyncLockLost 表示同步过程中锁已丢失，本次没有写入库存（HTTP 409）。
var ErrSyncLockLost = apperr.New(apperr.CodeSystemBusy, http.StatusConflict, "库存同步锁已丢失（平台调用过慢或被其它实例接管），本次未写库存，请重试")

// ErrBindingNotFound 表示绑定不存在（HTTP 404）。
var ErrBindingNotFound = apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "绑定不存在")

// ErrBindingConflict 表示券或平台 SKU 已被另一条绑定占用（HTTP 409）。
var ErrBindingConflict = apperr.New(apperr.CodeInvalidRequest, http.StatusConflict, "绑定冲突")

// ErrPlatformUnavailable 表示平台当前不可用（缺少凭证或熔断器打开，HTTP 503）。
var ErrPlatformUnavailable = apperr.New(apperr.CodeSystemBusy, http.StatusServiceUnavailable, "平台暂不可用")

// ErrPlatformRejected 表示平台以业务错误拒绝了请求，或返回了无法理解的响应（HTTP 502）。
var ErrPlatformRejected = apperr.New(apperr.CodeSystemBusy, http.StatusBadGateway, "平台拒绝了请求或返回了无法解析的响应")

// ErrPlatformSkuNotFound 表示平台上没有该 SKU（HTTP 404）。
var ErrPlatformSkuNotFound = apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "平台上未找到该 SKU")

// IsBindingNotFound 判断 err 是否为“绑定不存在”。ErrBindingNotFound 与 ErrBindingConflict 共用
// InvalidRequest 业务码（apperr 的 errors.Is 只按码判等），所以必须连同 HTTP 状态一起判断。
func IsBindingNotFound(err error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == ErrBindingNotFound.Code && e.HTTPStatus == http.StatusNotFound
}

// IsBindingConflict 判断 err 是否为“绑定冲突”，理由同 IsBindingNotFound。
func IsBindingConflict(err error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == ErrBindingConflict.Code && e.HTTPStatus == http.StatusConflict
}

// IsSyncBusy 判断 err 是否为同步锁被占用或中途丢失（两者都是 409 + SystemBusy 业务码）。
func IsSyncBusy(err error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == ErrSyncBusy.Code && e.HTTPStatus == http.StatusConflict
}
