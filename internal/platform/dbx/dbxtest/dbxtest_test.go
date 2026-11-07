package dbxtest

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestNewReturnsUsablePool(t *testing.T) {
	pool := New(t)
	var one int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("查询失败: one=%d err=%v", one, err)
	}
}

func TestNewIsolatesDatabases(t *testing.T) {
	ctx := context.Background()
	a, b := New(t), New(t)

	if _, err := a.Exec(ctx, "CREATE TABLE only_in_a (id int)"); err != nil {
		t.Fatal(err)
	}
	var existsInB bool
	if err := b.QueryRow(ctx, "SELECT to_regclass('only_in_a') IS NOT NULL").Scan(&existsInB); err != nil {
		t.Fatal(err)
	}
	if existsInB {
		t.Fatal("两个测试拿到的应是互相隔离的数据库")
	}
}

func TestCleanupDropsDatabase(t *testing.T) {
	ctx := context.Background()
	var dbName string
	t.Run("create", func(t *testing.T) {
		pool := New(t)
		if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
			t.Fatal(err)
		}
	})
	if dbName == "" {
		t.Fatal("未取到库名")
	}

	probe := New(t)
	var exists bool
	if err := probe.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatalf("子测试结束后数据库 %s 应被删除", dbName)
	}
}

func TestNewDSNPointsAtThrowawayDatabase(t *testing.T) {
	dsn := NewDSN(t)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "ck_t_") {
		t.Fatalf("DSN 应指向一次性库, got %s", u.Path)
	}
}

func TestWithDatabaseReplacesName(t *testing.T) {
	got, err := withDatabase("postgres://u:p@h:5432/postgres?sslmode=disable", "ck_t_x")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://u:p@h:5432/ck_t_x?sslmode=disable" {
		t.Fatalf("got %s", got)
	}
	if _, err := withDatabase("host=h dbname=x", "ck_t_x"); err == nil {
		t.Fatal("关键字形式 DSN 应被拒绝")
	}
}

func TestRequiredReadsEnv(t *testing.T) {
	t.Setenv(envRequire, "true")
	if !Required() {
		t.Fatal("CK_REQUIRE_PG=true 应返回 true")
	}
	t.Setenv(envRequire, "TRUE")
	if !Required() {
		t.Fatal("大小写不敏感")
	}
	t.Setenv(envRequire, "")
	if Required() {
		t.Fatal("未设置应返回 false")
	}
	t.Setenv(envRequire, "1")
	if Required() {
		t.Fatal("只有 true 才算")
	}
}
