// Package repo 是 user 服务的 PostgreSQL 实现（pgx），只访问 usr schema。
//
// 表名一律带 schema 前缀 usr.，不依赖连接的 search_path。本包实现 service.Repo。
package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// usernameConstraint 是 usr.users 上用户名唯一约束的名字（见 0001_init.sql）。
const usernameConstraint = "uk_users_username"

// 查询的公共列：phone/email 在库里可空，对外统一成空串。
const userColumns = `id, username, COALESCE(phone, ''), COALESCE(email, ''), role, status, last_active_at, created_at, updated_at`

// Repo 是基于 pgxpool 的 service.Repo 实现，并发安全。
type Repo struct {
	pool *pgxpool.Pool
}

// 编译期确认实现了 service 定义的接口。
var _ service.Repo = (*Repo)(nil)

// New 创建仓储。
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// scanUser 把 userColumns 的一行扫进 service.User。
func scanUser(row pgx.Row) (service.User, error) {
	var u service.User
	err := row.Scan(&u.ID, &u.Username, &u.Phone, &u.Email, &u.Role, &u.Status, &u.LastActiveAt, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// Create 插入用户并返回库里的记录。用户名唯一约束冲突返回 service.ErrUsernameTaken；
// 其他约束（主键、角色 CHECK 等）冲突原样包装返回，它们代表程序缺陷而不是用户输入问题。
func (r *Repo) Create(ctx context.Context, a service.NewAccount) (service.User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO usr.users (id, username, password_hash, phone, email, role)
		 VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
		 RETURNING `+userColumns,
		a.ID, a.Username, a.PasswordHash, a.Phone, a.Email, a.Role)
	u, err := scanUser(row)
	switch {
	case dbx.IsUniqueViolation(err, usernameConstraint):
		return service.User{}, service.ErrUsernameTaken
	case err != nil:
		return service.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// FindByUsername 按用户名（区分大小写）取完整记录，不存在返回 service.ErrNotFound。
func (r *Repo) FindByUsername(ctx context.Context, username string) (service.Account, error) {
	var acc service.Account
	err := r.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM usr.users WHERE username = $1`, username).
		Scan(&acc.ID, &acc.Username, &acc.Phone, &acc.Email, &acc.Role, &acc.Status,
			&acc.LastActiveAt, &acc.CreatedAt, &acc.UpdatedAt, &acc.PasswordHash)
	switch {
	case dbx.IsNoRows(err):
		return service.Account{}, service.ErrNotFound
	case err != nil:
		return service.Account{}, fmt.Errorf("select user by username: %w", err)
	}
	return acc, nil
}

// FindByID 按 ID 取用户视图，不存在返回 service.ErrNotFound。
func (r *Repo) FindByID(ctx context.Context, id int64) (service.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM usr.users WHERE id = $1`, id))
	switch {
	case dbx.IsNoRows(err):
		return service.User{}, service.ErrNotFound
	case err != nil:
		return service.User{}, fmt.Errorf("select user by id: %w", err)
	}
	return u, nil
}

// TouchLastActive 仅当 last_active_at 为空或距今不小于 minInterval 时把它更新为 now。
// 条件与更新在同一条 UPDATE 里，由数据库原子判断：多个实例并发登录同一用户，至多一个得到 true。
func (r *Repo) TouchLastActive(ctx context.Context, id int64, now time.Time, minInterval time.Duration) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE usr.users SET last_active_at = $2
		 WHERE id = $1
		   AND (last_active_at IS NULL OR last_active_at <= $2::timestamptz - make_interval(secs => $3::float8))`,
		id, now, minInterval.Seconds())
	if err != nil {
		return false, fmt.Errorf("touch last_active_at: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// InsertBatch 用一条 INSERT ... SELECT unnest(...) 插入一批用户；用户名已存在的行被跳过
// （ON CONFLICT (username) DO NOTHING，只吞用户名冲突，主键等其他冲突仍会报错）。返回实际插入的行数。
func (r *Repo) InsertBatch(ctx context.Context, batch []service.NewAccount) (int, error) {
	if len(batch) == 0 {
		return 0, nil
	}
	n := len(batch)
	ids, names, hashes := make([]int64, n), make([]string, n), make([]string, n)
	phones, emails, roles := make([]string, n), make([]string, n), make([]string, n)
	for i, a := range batch {
		ids[i], names[i], hashes[i] = a.ID, a.Username, a.PasswordHash
		phones[i], emails[i], roles[i] = a.Phone, a.Email, a.Role
	}
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO usr.users (id, username, password_hash, phone, email, role)
		 SELECT t.id, t.username, t.hash, NULLIF(t.phone, ''), NULLIF(t.email, ''), t.role
		 FROM unnest($1::bigint[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
		      AS t(id, username, hash, phone, email, role)
		 ON CONFLICT (username) DO NOTHING`,
		ids, names, hashes, phones, emails, roles)
	if err != nil {
		return 0, fmt.Errorf("insert user batch of %d: %w", n, err)
	}
	return int(tag.RowsAffected()), nil
}

// SeedDemo 执行内嵌的演示种子（demo/demo1234、admin/admin1234），幂等。
// 仅应在 CK_USER_SEED_DEMO / CK_SEED_DEMO 为 true 且非生产环境时调用。
func (r *Repo) SeedDemo(ctx context.Context) error {
	sql, err := migrations.FS.ReadFile(migrations.SeedDemoFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", migrations.SeedDemoFile, err)
	}
	if _, err := r.pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("execute %s: %w", migrations.SeedDemoFile, err)
	}
	return nil
}
