// Package dbxtest 为需要真实 PostgreSQL 的测试提供隔离的数据库。
//
// 服务器来源（按优先级）：
//  1. 环境变量 CK_TEST_PG_DSN 指向的服务器（URL 形式，例如 postgres://user:pass@host:5432/postgres）。
//     每个测试在其中创建一个一次性数据库，t.Cleanup 时强制删除。CI 应使用这一种。
//  2. 内嵌 PostgreSQL 16（github.com/fergusstrange/embedded-postgres）：首次使用时按需下载二进制到
//     用户缓存目录，端口动态选择，同一个测试进程内只启动一次，进程退出后由守护子进程停止并清理。
//  3. 两者都不可用：t.Skip 并给出原因；若设置了 CK_REQUIRE_PG=true（见 Required），则改为 t.Fatal，
//     避免 CI 因跳过而“绿”。
package dbxtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
)

const (
	envDSN     = "CK_TEST_PG_DSN"
	envRequire = "CK_REQUIRE_PG"

	adminTimeout = 30 * time.Second
)

// Required 报告当前环境是否要求必须有 PostgreSQL 可用（CK_REQUIRE_PG=true）。
// 为 true 时，PostgreSQL 不可用会让测试失败而不是跳过。
func Required() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(envRequire)), "true")
}

var (
	serverOnce sync.Once
	serverDSN  string
	serverErr  error
)

// server 惰性解析并缓存测试用 PostgreSQL 服务器的管理 DSN（进程内只解析一次）。
func server() (string, error) {
	serverOnce.Do(func() {
		if dsn := strings.TrimSpace(os.Getenv(envDSN)); dsn != "" {
			serverDSN = dsn
			return
		}
		serverDSN, serverErr = startEmbedded()
	})
	return serverDSN, serverErr
}

// unavailable 在 PostgreSQL 不可用时按 Required() 决定失败或跳过。
func unavailable(t testing.TB, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	if Required() {
		t.Fatalf("PostgreSQL 不可用且 %s=true: %s", envRequire, msg)
	}
	t.Skipf("PostgreSQL 不可用，跳过（设置 %s 指向 PG，或设置 %s=true 让其失败）: %s", envDSN, envRequire, msg)
}

// NewDSN 创建一个一次性数据库并返回指向它的 DSN（URL 形式），t.Cleanup 时强制删除该库。
// 适用于被测代码自己调用 dbx.Open 的场景。PostgreSQL 不可用时跳过或失败，见包文档。
func NewDSN(t testing.TB) string {
	t.Helper()
	admin, err := server()
	if err != nil {
		unavailable(t, "%v", err)
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		if os.Getenv(envDSN) != "" {
			t.Fatalf("连接 %s 指定的 PostgreSQL 失败: %v", envDSN, err)
		}
		unavailable(t, "连接内嵌 PostgreSQL 失败: %v", err)
		return ""
	}
	defer func() { _ = conn.Close(context.Background()) }()

	name, err := randomDBName()
	if err != nil {
		t.Fatalf("生成测试库名: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("创建测试数据库 %s: %v", name, err)
	}
	t.Cleanup(func() { dropDatabase(t, admin, name) })

	dsn, err := withDatabase(admin, name)
	if err != nil {
		t.Fatalf("构造测试库 DSN: %v", err)
	}
	return dsn
}

// New 创建一个一次性数据库并返回连接到它的连接池；连接池与数据库都在 t.Cleanup 时释放。
// 不同测试得到互相隔离的数据库，可并行运行。PostgreSQL 不可用时跳过或失败，见包文档。
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := NewDSN(t)
	pool, err := dbx.Open(context.Background(), dsn, dbx.Options{
		MaxConns:           8,
		MinConns:           1,
		SlowQueryThreshold: -1,
		ApplicationName:    "dbxtest",
	})
	if err != nil {
		t.Fatalf("打开测试库连接池: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// dropDatabase 强制删除一次性数据库（PG13+ 的 WITH (FORCE) 会踢掉残留连接）。
func dropDatabase(t testing.TB, admin, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Logf("清理测试库 %s: 连接失败: %v", name, err)
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		t.Logf("清理测试库 %s: %v", name, err)
	}
}

// randomDBName 生成形如 ck_t_<16 位十六进制> 的库名。
func randomDBName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("读取随机数: %w", err)
	}
	return "ck_t_" + hex.EncodeToString(b), nil
}

// withDatabase 把 URL 形式 DSN 的库名替换为 db。
func withDatabase(dsn, db string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("%s 必须是 postgres:// URL 形式的 DSN: %v", envDSN, err)
	}
	u.Path = "/" + db
	u.RawPath = ""
	return u.String(), nil
}
