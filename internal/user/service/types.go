// Package service 实现 user 服务的业务规则：注册校验、登录（防枚举/防时序）、登出、资料查询与压测造用户。
//
// 本包只依赖自己定义的小接口（Repo、Sessions、Tokens、PasswordHasher、IDGenerator），
// 因此全部规则都能脱离 PostgreSQL/Redis 直接单测；真实实现分别在 internal/user/repo 与 platform/redisx。
package service

import (
	"context"
	"errors"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
)

// 用户状态取值，与 usr.users 的 CHECK 约束一致。
const (
	// StatusDisabled 表示账号被停用，不能登录。
	StatusDisabled = 0
	// StatusActive 表示账号正常。
	StatusActive = 1
)

// ErrUsernameTaken 由 Repo 在用户名唯一约束冲突时返回，service 把它转成 apperr.UserExist。
var ErrUsernameTaken = errors.New("user: username already taken")

// ErrNotFound 由 Repo 在按用户名或 ID 查不到用户时返回。
var ErrNotFound = errors.New("user: not found")

// User 是对外可见的用户视图：刻意不含口令哈希，服务层之外永远拿不到哈希。
type User struct {
	// ID 是雪花 ID。
	ID int64
	// Username 是全局唯一的登录名。
	Username string
	// Phone 是手机号，可为空（压测用户或历史数据）。
	Phone string
	// Email 是邮箱，可为空。
	Email string
	// Role 是 "user" 或 "admin"。
	Role string
	// Status 是 StatusDisabled 或 StatusActive。
	Status int
	// LastActiveAt 是最近一次登录刷新的时间，从未登录过则为 nil。
	LastActiveAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// Account 是仓储层返回的完整记录：在 User 之外带有口令哈希，仅在 service 内部使用。
type Account struct {
	User
	// PasswordHash 是 bcrypt 哈希。
	PasswordHash string
}

// NewAccount 是待插入的用户。
type NewAccount struct {
	// ID 是应用生成的雪花 ID。
	ID int64
	// Username 是登录名。
	Username string
	// PasswordHash 是 bcrypt 哈希。
	PasswordHash string
	// Phone 是手机号。
	Phone string
	// Email 是邮箱，可为空。
	Email string
	// Role 是 "user" 或 "admin"。
	Role string
}

// Repo 是 service 依赖的持久化接口，由 internal/user/repo 实现。
type Repo interface {
	// Create 插入一个用户并返回数据库里的记录；用户名冲突返回 ErrUsernameTaken。
	Create(ctx context.Context, a NewAccount) (User, error)
	// FindByUsername 按用户名取完整记录；不存在返回 ErrNotFound。
	FindByUsername(ctx context.Context, username string) (Account, error)
	// FindByID 按 ID 取用户视图；不存在返回 ErrNotFound。
	FindByID(ctx context.Context, id int64) (User, error)
	// TouchLastActive 在 last_active_at 为空或早于 now-minInterval 时把它置为 now，
	// 返回是否真的更新了；该条件由数据库原子判断，多实例并发登录也不会突破频率上限。
	TouchLastActive(ctx context.Context, id int64, now time.Time, minInterval time.Duration) (bool, error)
	// InsertBatch 一条语句插入一批用户，用户名冲突的行被跳过，返回实际插入的行数。
	InsertBatch(ctx context.Context, batch []NewAccount) (int, error)
}

// Sessions 是登录会话存储（ck:sess:{jti}），*redisx.SessionStore 与 authx.MemoryRevoker 均满足。
type Sessions interface {
	// Open 登记会话，ttl 为令牌剩余寿命。
	Open(ctx context.Context, jti string, ttl time.Duration) error
	// Revoke 删除会话，只影响这一个 jti；不存在不是错误。
	Revoke(ctx context.Context, jti string) error
}

// Tokens 是 JWT 签发与校验，*authx.Tokens 满足。
type Tokens interface {
	// Issue 为身份签发令牌。
	Issue(id authx.Identity) (authx.Token, error)
	// Verify 校验令牌并返回会话信息。
	Verify(raw string) (authx.Session, error)
}

// PasswordHasher 抽象口令哈希，便于测试统计调用次数与跳过 bcrypt 的开销。
type PasswordHasher interface {
	// Hash 生成口令哈希。
	Hash(plain string) (string, error)
	// VerifyOrDummy 校验口令；hash 为空串（用户不存在）时做等价工作量的哑比较并返回 false。
	VerifyOrDummy(hash, plain string) bool
}

// IDGenerator 生成用户 ID，*idgen.Generator 满足。
type IDGenerator interface {
	// Next 返回下一个唯一 ID。
	Next() int64
}

// AuthxHasher 是基于 authx（bcrypt cost 10）的 PasswordHasher 默认实现。
type AuthxHasher struct{}

// Hash 实现 PasswordHasher。
func (AuthxHasher) Hash(plain string) (string, error) { return authx.HashPassword(plain) }

// VerifyOrDummy 实现 PasswordHasher。
func (AuthxHasher) VerifyOrDummy(hash, plain string) bool { return authx.VerifyOrDummy(hash, plain) }
