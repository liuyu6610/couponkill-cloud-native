package dbx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrChecksumMismatch 表示某个已应用的迁移文件在应用之后被改动过。
// 已发布的迁移必须是不可变的：要改表结构请新增一个更大版本号的迁移文件。
var ErrChecksumMismatch = errors.New("已应用的迁移文件被修改")

var (
	// migrationFileRE 匹配合法的迁移文件名：四位版本号 + 下划线 + 小写蛇形名 + .sql。
	migrationFileRE = regexp.MustCompile(`^(\d{4})_([a-z0-9]+(?:_[a-z0-9]+)*)\.sql$`)
	// numberedPrefixRE 匹配“看起来想当迁移文件”的名字（以数字开头），用来拦截拼写错误而不是悄悄忽略。
	numberedPrefixRE = regexp.MustCompile(`^\d`)
	schemaNameRE     = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
)

const (
	lockPollInterval = 100 * time.Millisecond
	unlockTimeout    = 5 * time.Second
)

// migration 是一个已解析的迁移文件。
type migration struct {
	version  int64
	name     string
	checksum string
	sql      string
}

// Migrate 把 fsys 根目录下形如 NNNN_name.sql 的迁移文件按版本号升序应用到 schema：
//
//   - schema 不存在则创建；已应用的版本记录在 <schema>.schema_migrations(version, name, checksum, applied_at)。
//   - 全程持有按 schema 区分的 PostgreSQL advisory lock，多个服务副本同时启动是安全的：
//     后到者等待，拿到锁后发现已全部应用，什么也不做。
//   - 每个文件在独立事务里执行（文件内容与版本记录同事务提交），失败则该文件整体回滚、后续文件不再执行。
//   - 文件内使用不带 schema 前缀的名字：事务内 search_path 被设为目标 schema。
//   - 幂等：重复运行是空操作。已应用文件的内容（忽略换行风格）与记录的 SHA-256 不一致时，
//     在应用任何新迁移之前返回 ErrChecksumMismatch。
//   - 不符合 NNNN_name.sql 的文件（如 seed_demo.sql）被忽略；以数字开头但格式不对的文件会报错。
//
// 库里存在但 fsys 里已没有的版本（例如回滚到旧版本二进制）只记 Warn 日志，不阻止启动。
func Migrate(ctx context.Context, pool *pgxpool.Pool, schema string, fsys fs.FS) error {
	if err := validateSchema(schema); err != nil {
		return err
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("读取 %s 的迁移文件: %w", schema, err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("为迁移获取连接: %w", err)
	}
	defer conn.Release()

	key := advisoryKey(schema)
	if err := acquireAdvisoryLock(ctx, conn, key); err != nil {
		return fmt.Errorf("获取 %s 的迁移锁: %w", schema, err)
	}
	defer releaseAdvisoryLock(ctx, conn, key, schema)

	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoted); err != nil {
		return fmt.Errorf("创建 schema %s: %w", schema, err)
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+quoted+`.schema_migrations (
		version    BIGINT      PRIMARY KEY,
		name       TEXT        NOT NULL,
		checksum   TEXT        NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("创建 %s.schema_migrations: %w", schema, err)
	}

	applied, err := loadApplied(ctx, conn, quoted)
	if err != nil {
		return fmt.Errorf("读取 %s 的迁移记录: %w", schema, err)
	}
	pending, err := planPending(schema, migs, applied)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := applyOne(ctx, conn, schema, quoted, m); err != nil {
			return err
		}
		slog.InfoContext(ctx, "已应用数据库迁移", "schema", schema, "version", m.version, "name", m.name)
	}
	return nil
}

// planPending 先校验所有已应用文件的校验和（任一不符即失败，此时尚未改动数据库），再返回待应用的迁移。
func planPending(schema string, migs []migration, applied map[int64]string) ([]migration, error) {
	known := make(map[int64]bool, len(migs))
	var pending []migration
	for _, m := range migs {
		known[m.version] = true
		sum, done := applied[m.version]
		switch {
		case !done:
			pending = append(pending, m)
		case sum != m.checksum:
			return nil, fmt.Errorf("%w: schema=%s version=%04d name=%s", ErrChecksumMismatch, schema, m.version, m.name)
		}
	}
	for v := range applied {
		if !known[v] {
			slog.Warn("数据库里有当前程序不认识的迁移版本（可能是回滚到了旧版本）", "schema", schema, "version", v)
		}
	}
	return pending, nil
}

// applyOne 在一个事务里执行迁移文件并记录版本。
func applyOne(ctx context.Context, conn *pgxpool.Conn, schema, quoted string, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("迁移 %s/%04d_%s 开启事务: %w", schema, m.version, m.name, err)
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", schema); err != nil {
		return fmt.Errorf("迁移 %s/%04d_%s 设置 search_path: %w", schema, m.version, m.name, err)
	}
	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return fmt.Errorf("迁移 %s/%04d_%s 执行失败: %w", schema, m.version, m.name, err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO "+quoted+".schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
		m.version, m.name, m.checksum); err != nil {
		return fmt.Errorf("迁移 %s/%04d_%s 记录版本: %w", schema, m.version, m.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("迁移 %s/%04d_%s 提交: %w", schema, m.version, m.name, err)
	}
	return nil
}

// loadApplied 读取已应用版本 → 校验和。
func loadApplied(ctx context.Context, conn *pgxpool.Conn, quoted string) (map[int64]string, error) {
	rows, err := conn.Query(ctx, "SELECT version, checksum FROM "+quoted+".schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := make(map[int64]string)
	for rows.Next() {
		var v int64
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, err
		}
		applied[v] = sum
	}
	return applied, rows.Err()
}

// acquireAdvisoryLock 以轮询 pg_try_advisory_lock 的方式获取会话级咨询锁：
// 不会被 statement_timeout 打断，并且随 ctx 取消及时退出。
func acquireAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, key int64) error {
	for {
		var ok bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
			return err
		}
		if ok {
			return nil
		}
		if err := sleepCtx(ctx, lockPollInterval); err != nil {
			return err
		}
	}
}

// releaseAdvisoryLock 释放咨询锁。若释放失败（连接已坏等），把连接从池里摘掉并关闭，
// 避免带着锁的会话回到连接池后让别的副本永远拿不到锁。
func releaseAdvisoryLock(ctx context.Context, conn *pgxpool.Conn, key int64, schema string) {
	unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
	defer cancel()
	var released bool
	err := conn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock($1)", key).Scan(&released)
	if err == nil && released {
		return
	}
	slog.Warn("释放迁移咨询锁失败，关闭该连接", "schema", schema, "error", err, "released", released)
	raw := conn.Hijack()
	if closeErr := raw.Close(unlockCtx); closeErr != nil {
		slog.Warn("关闭迁移连接失败", "schema", schema, "error", closeErr)
	}
}

// advisoryKey 由 schema 名派生稳定的 64 位咨询锁键，不同 schema 的迁移互不阻塞。
func advisoryKey(schema string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("couponkill:dbx:migrate:" + schema))
	return int64(h.Sum64())
}

// validateSchema 只允许小写蛇形标识符，既防注入也保证无需引号转义的稳定写法。
func validateSchema(schema string) error {
	if !schemaNameRE.MatchString(schema) {
		return fmt.Errorf("非法的 schema 名 %q：只允许小写字母、数字、下划线且不以数字开头", schema)
	}
	return nil
}

// loadMigrations 读取并校验 fsys 根目录下的迁移文件，按版本号数值升序返回。
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("列出迁移目录: %w", err)
	}
	seen := make(map[int64]string)
	var migs []migration
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		sub := migrationFileRE.FindStringSubmatch(name)
		if sub == nil {
			if numberedPrefixRE.MatchString(name) {
				return nil, fmt.Errorf("迁移文件名 %q 不符合 NNNN_snake_name.sql", name)
			}
			continue
		}
		version, err := strconv.ParseInt(sub[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析迁移版本 %q: %w", name, err)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("迁移版本 %04d 重复: %s 与 %s", version, prev, name)
		}
		seen[version] = name

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("读取迁移文件 %s: %w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("迁移文件 %s 是空的", name)
		}
		migs = append(migs, migration{version: version, name: sub[2], checksum: checksum(data), sql: string(data)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// checksum 返回迁移内容的 SHA-256（十六进制）。先把 CRLF 归一为 LF，
// 这样 Windows 上 git autocrlf 检出的文件不会被误判为“被修改过”。
func checksum(data []byte) string {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
