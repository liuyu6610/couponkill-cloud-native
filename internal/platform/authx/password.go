package authx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 固定为 10（design §5）。
const bcryptCost = 10

// HashPassword 用 bcrypt（cost 10）哈希口令。超过 bcrypt 的 72 字节上限会报错而不是被静默截断。
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("authx: hash password: %w", err)
	}
	return string(h), nil
}

// VerifyPassword 校验口令；哈希为空或格式非法一律返回 false。
func VerifyPassword(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// VerifyOrDummy 在 hash 为空（用户不存在）时做一次等价工作量的 bcrypt 比较并返回 false，
// 使“用户不存在”与“口令错误”在耗时上不可区分（防用户枚举的时序侧信道）。
func VerifyOrDummy(hash, plain string) bool {
	if hash != "" {
		return VerifyPassword(hash, plain)
	}
	dummyOnce.Do(func() {
		var b [16]byte
		_, _ = rand.Read(b[:])
		// 32 字符的随机输入低于 72 字节上限，哈希不会失败。
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(b[:])), bcryptCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
	return false
}
