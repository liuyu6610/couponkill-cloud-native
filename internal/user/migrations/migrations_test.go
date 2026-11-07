package migrations_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/migrations"
)

const schema = "usr"

func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, schema, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	return err
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func TestMigrationsApplyFromEmptyAndTwiceIsNoOp(t *testing.T) {
	pool := migrated(t)
	files, err := fs.Glob(migrations.FS, "[0-9][0-9][0-9][0-9]_*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v %v", files, err)
	}
	if err := dbx.Migrate(context.Background(), pool, schema, migrations.FS); err != nil {
		t.Fatalf("第二次应用应是空操作: %v", err)
	}
	if n := count(t, pool, "SELECT count(*) FROM usr.schema_migrations"); n != len(files) {
		t.Fatalf("版本记录数 %d != 文件数 %d", n, len(files))
	}
}

func TestEditedAppliedMigrationIsDetected(t *testing.T) {
	pool := migrated(t)
	edited := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.FS, ".")
	for _, e := range entries {
		data, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(e.Name(), "0001_") {
			data = append(data, []byte("\n-- sneaky edit\n")...)
		}
		edited[e.Name()] = &fstest.MapFile{Data: data}
	}
	err := dbx.Migrate(context.Background(), pool, schema, edited)
	if err == nil || !strings.Contains(err.Error(), dbx.ErrChecksumMismatch.Error()) {
		t.Fatalf("改动已应用的迁移应被检测到, got %v", err)
	}
}

func TestNoForeignKeysLeaveTheSchema(t *testing.T) {
	pool := migrated(t)
	n := count(t, pool, `SELECT count(*) FROM pg_constraint c
		JOIN pg_class a ON a.oid = c.conrelid JOIN pg_class b ON b.oid = c.confrelid
		WHERE c.contype = 'f' AND a.relnamespace <> b.relnamespace`)
	if n != 0 {
		t.Fatalf("不得有跨 schema 外键, got %d", n)
	}
}

const insertUser = `INSERT INTO usr.users (id, username, password_hash, role) VALUES ($1, $2, 'h', $3)`

func TestUsernameIsGloballyUnique(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, insertUser, int64(1), "alice", "user"); err != nil {
		t.Fatal(err)
	}
	err := exec(t, pool, insertUser, int64(2), "alice", "user")
	if !dbx.IsUniqueViolation(err, "uk_users_username") {
		t.Fatalf("重复用户名应违反 uk_users_username, got %v", err)
	}
}

func TestRoleAndStatusAreConstrained(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, insertUser, int64(1), "root", "superuser"); !dbx.IsCheckViolation(err, "ck_users_role") {
		t.Fatalf("未知角色应被拒绝: %v", err)
	}
	if err := exec(t, pool, insertUser, int64(2), "boss", "admin"); err != nil {
		t.Fatalf("admin 角色应允许: %v", err)
	}
	if err := exec(t, pool, "INSERT INTO usr.users (id, username, password_hash, status) VALUES (3, 'x', 'h', 7)"); !dbx.IsCheckViolation(err, "ck_users_status") {
		t.Fatalf("非法状态应被拒绝: %v", err)
	}
	if err := exec(t, pool, insertUser, int64(4), strings.Repeat("a", 51), "user"); !dbx.IsCheckViolation(err, "ck_users_username_len") {
		t.Fatalf("超长用户名应被拒绝: %v", err)
	}
	if err := exec(t, pool, insertUser, int64(5), "", "user"); !dbx.IsCheckViolation(err, "ck_users_username_len") {
		t.Fatalf("空用户名应被拒绝: %v", err)
	}
}

func TestNewUserDefaults(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, "INSERT INTO usr.users (id, username, password_hash) VALUES (1, 'bob', 'h')"); err != nil {
		t.Fatal(err)
	}
	var role string
	var status int
	var lastActive *string
	var createdSet bool
	err := pool.QueryRow(context.Background(),
		"SELECT role, status, last_active_at::text, created_at IS NOT NULL FROM usr.users WHERE id = 1").
		Scan(&role, &status, &lastActive, &createdSet)
	if err != nil {
		t.Fatal(err)
	}
	if role != "user" || status != 1 || lastActive != nil || !createdSet {
		t.Fatalf("默认值不对: role=%s status=%d lastActive=%v created=%v", role, status, lastActive, createdSet)
	}
}

func readSeed(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(migrations.FS, migrations.SeedDemoFile)
	if err != nil {
		t.Fatalf("读取种子文件: %v", err)
	}
	return string(b)
}

func TestSeedIsNotPartOfMigrations(t *testing.T) {
	pool := migrated(t)
	if n := count(t, pool, "SELECT count(*) FROM usr.users"); n != 0 {
		t.Fatalf("迁移不得写入演示用户, got %d", n)
	}
}

func TestSeedDemoCreatesVerifiableAccountsAndIsIdempotent(t *testing.T) {
	pool := migrated(t)
	seed := readSeed(t)

	for i := 0; i < 2; i++ { // 执行两次：幂等
		if err := exec(t, pool, seed); err != nil {
			t.Fatalf("第 %d 次执行种子: %v", i+1, err)
		}
	}
	if n := count(t, pool, "SELECT count(*) FROM usr.users"); n != 2 {
		t.Fatalf("应恰好有 2 个种子用户, got %d", n)
	}

	cases := []struct{ username, password, role string }{
		{"demo", "demo1234", "user"},
		{"admin", "admin1234", "admin"},
	}
	for _, c := range cases {
		var hash, role string
		if err := pool.QueryRow(context.Background(),
			"SELECT password_hash, role FROM usr.users WHERE username = $1", c.username).Scan(&hash, &role); err != nil {
			t.Fatalf("%s: %v", c.username, err)
		}
		if role != c.role {
			t.Errorf("%s 角色应为 %s, got %s", c.username, c.role, role)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(c.password)); err != nil {
			t.Errorf("%s 的 bcrypt 哈希无法校验口令 %s: %v", c.username, c.password, err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong-password")); err == nil {
			t.Errorf("%s 的哈希不应匹配错误口令", c.username)
		}
		if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != 10 {
			t.Errorf("%s 的 bcrypt cost 应为 10: cost=%d err=%v", c.username, cost, err)
		}
	}
}

func TestSeedDoesNotOverwriteExistingUser(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, "INSERT INTO usr.users (id, username, password_hash) VALUES (777, 'demo', 'changed-by-user')"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, readSeed(t)); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := pool.QueryRow(context.Background(), "SELECT password_hash FROM usr.users WHERE username = 'demo'").Scan(&hash); err != nil || hash != "changed-by-user" {
		t.Fatalf("种子不得覆盖已存在的用户: hash=%q err=%v", hash, err)
	}
}
