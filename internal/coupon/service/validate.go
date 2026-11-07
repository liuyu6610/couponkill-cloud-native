package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 字段上限（与迁移里的 CHECK 约束保持一致或更严）。
const (
	maxNameRunes        = 100
	maxDescriptionRunes = 500
	maxValidDays        = 3650
	maxPerUserLimit     = 1000
)

// moneyRE 匹配最多 8 位整数、最多 2 位小数的非负金额。
var moneyRE = regexp.MustCompile(`^(\d{1,8})(?:\.(\d{1,2}))?$`)

// NormalizeMoney 把金额文本规范化为两位小数（"50" -> "50.00"，"7.5" -> "7.50"）。
// 拒绝负数、科学计数法、超过两位小数或整数部分超过 8 位的输入，金额全程不经过浮点数。
func NormalizeMoney(s string) (string, error) {
	m := moneyRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", fmt.Errorf("金额 %q 不合法：需为非负数，最多两位小数", s)
	}
	frac := m[2]
	for len(frac) < 2 {
		frac += "0"
	}
	return m[1] + "." + frac, nil
}

// jsonNumber 把已规范化的十进制文本包成 json.Number（内部契约里金额保持原样文本）。
func jsonNumber(s string) json.Number { return json.Number(s) }

func invalidf(format string, args ...any) error {
	return apperr.InvalidRequest.WithMessage("请求参数错误: " + fmt.Sprintf(format, args...))
}

// validateCreate 校验 CreateCommand 并生成可落库的 NewCoupon。
func (s *Service) validateCreate(cmd CreateCommand) (NewCoupon, error) {
	name := strings.TrimSpace(cmd.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameRunes {
		return NewCoupon{}, invalidf("name 长度必须为 1 到 %d 个字符", maxNameRunes)
	}
	desc := strings.TrimSpace(cmd.Description)
	if utf8.RuneCountInString(desc) > maxDescriptionRunes {
		return NewCoupon{}, invalidf("description 不能超过 %d 个字符", maxDescriptionRunes)
	}
	if cmd.Type != TypeNormal && cmd.Type != TypeSeckill {
		return NewCoupon{}, invalidf("type 只能是 1（常驻）或 2（秒杀）")
	}
	face, err := NormalizeMoney(cmd.FaceValue)
	if err != nil {
		return NewCoupon{}, invalidf("faceValue: %v", err)
	}
	minSpend, err := NormalizeMoney(cmd.MinSpend)
	if err != nil {
		return NewCoupon{}, invalidf("minSpend: %v", err)
	}
	if cmd.ValidDays < 1 || cmd.ValidDays > maxValidDays {
		return NewCoupon{}, invalidf("validDays 必须为 1 到 %d", maxValidDays)
	}
	if cmd.PerUserLimit < 1 || cmd.PerUserLimit > maxPerUserLimit {
		return NewCoupon{}, invalidf("perUserLimit 必须为 1 到 %d", maxPerUserLimit)
	}
	if cmd.ID < 0 {
		return NewCoupon{}, invalidf("id 必须为正数")
	}

	total := cmd.TotalStock
	if total < 0 || total > MaxStock {
		return NewCoupon{}, invalidf("库存必须为 0 到 %d", MaxStock)
	}

	if err := validateWindow(cmd); err != nil {
		return NewCoupon{}, err
	}

	id := cmd.ID
	if id == 0 {
		id = s.ids.Next()
	}
	return NewCoupon{
		ID: id, Name: name, Description: desc, Type: cmd.Type,
		FaceValue: face, MinSpend: minSpend, ValidDays: cmd.ValidDays, PerUserLimit: cmd.PerUserLimit,
		TotalStock: total, SeckillStartAt: cmd.SeckillStartAt, SeckillEndAt: cmd.SeckillEndAt,
	}, nil
}

// validateWindow 校验时间窗：秒杀券必须成对给出且 start 早于 end；常驻券不得设置时间窗。
func validateWindow(cmd CreateCommand) error {
	hasStart, hasEnd := cmd.SeckillStartAt != nil, cmd.SeckillEndAt != nil
	if cmd.Type == TypeNormal {
		if hasStart || hasEnd {
			return invalidf("仅秒杀券可设置活动时间窗")
		}
		return nil
	}
	if !hasStart || !hasEnd {
		return apperr.New(apperr.CodeInvalidRequest, http.StatusBadRequest, "请求参数错误: 秒杀券必须提供 seckillStartAt / seckillEndAt")
	}
	if !cmd.SeckillStartAt.Before(*cmd.SeckillEndAt) {
		return invalidf("seckillStartAt 必须早于 seckillEndAt")
	}
	return nil
}
