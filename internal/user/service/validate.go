package service

import (
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 校验常量。规则移植自 Java UserController.doRegister，并补上 Java 没有的上限：
// 用户名上限对应表的 CHECK(char_length BETWEEN 1 AND 50)，口令上限是 bcrypt 只看前 72 字节的硬限制
// （超出会被静默截断或报错，宁可明确拒绝）。
const (
	// MaxUsernameRunes 是用户名最大字符数。
	MaxUsernameRunes = 50
	// MinPasswordRunes 是口令最小字符数（Java：不少于 6 位）。
	MinPasswordRunes = 6
	// MaxPasswordBytes 是口令最大字节数（bcrypt 限制）。
	MaxPasswordBytes = 72
)

// phonePattern 是中国大陆手机号（Java: ^1[3-9]\d{9}$）。显式写 [0-9] 以拒绝全角等 Unicode 数字；
// RE2 的 $ 在非多行模式下只匹配文本末尾，因此结尾带换行的输入同样被拒绝。
var phonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

func invalid(msg string) error {
	return apperr.InvalidRequest.WithMessage("请求参数错误: " + msg)
}

// validateUsername 要求 1..50 个字符，且不含任何空白或控制字符：
// 首尾空格会造成视觉上相同却互不相等的“影子账号”，内部空白与控制字符在日志和 UI 里都有歧义。
func validateUsername(name string) error {
	if name == "" {
		return invalid("用户名不能为空")
	}
	n := utf8.RuneCountInString(name)
	if n > MaxUsernameRunes {
		return invalid("用户名不能超过 50 个字符")
	}
	if !utf8.ValidString(name) {
		return invalid("用户名包含非法字符")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return invalid("用户名不能包含空白或控制字符")
		}
	}
	return nil
}

// validateNewPassword 用于注册：至少 6 个字符、至多 72 字节。
func validateNewPassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordRunes {
		return invalid("密码长度不能少于 6 位")
	}
	if len(pw) > MaxPasswordBytes {
		return invalid("密码长度不能超过 72 字节")
	}
	return nil
}

// validatePhone 要求大陆手机号格式。
func validatePhone(phone string) error {
	if !phonePattern.MatchString(phone) {
		return invalid("手机号格式不正确")
	}
	return nil
}
