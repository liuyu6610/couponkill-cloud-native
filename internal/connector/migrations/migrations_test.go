package migrations_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/connector/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

const schema = "cnx"

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

const insertBinding = `INSERT INTO cnx.platform_sku_bindings (id, platform, external_sku_id, coupon_id) VALUES ($1, $2, $3, $4)`

func TestMigrationsApplyFromEmptyAndTwiceIsNoOp(t *testing.T) {
	pool := migrated(t)
	files, err := fs.Glob(migrations.FS, "[0-9][0-9][0-9][0-9]_*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v %v", files, err)
	}
	if err := dbx.Migrate(context.Background(), pool, schema, migrations.FS); err != nil {
		t.Fatalf("第二次应用应是空操作: %v", err)
	}
	if n := count(t, pool, "SELECT count(*) FROM cnx.schema_migrations"); n != len(files) {
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

func TestBindingUniqueness(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, insertBinding, int64(1), "JD", "sku-1", int64(1001)); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, insertBinding, int64(2), "JD", "sku-1", int64(1002)); !dbx.IsUniqueViolation(err, "uk_bindings_platform_sku") {
		t.Fatalf("同平台同 SKU 不能绑定两次: %v", err)
	}
	if err := exec(t, pool, insertBinding, int64(3), "PDD", "sku-9", int64(1001)); !dbx.IsUniqueViolation(err, "uk_bindings_coupon") {
		t.Fatalf("一张券只能绑一个 SKU: %v", err)
	}
	if err := exec(t, pool, insertBinding, int64(4), "PDD", "sku-1", int64(1003)); err != nil {
		t.Fatalf("不同平台可用相同的 SKU 号: %v", err)
	}
}

func TestBindingChecks(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, insertBinding, int64(1), "jd", "s", int64(1)); !dbx.IsCheckViolation(err, "ck_bindings_platform") {
		t.Fatalf("平台名必须是大写标识: %v", err)
	}
	if err := exec(t, pool, insertBinding, int64(2), "JD", "  ", int64(2)); !dbx.IsCheckViolation(err, "ck_bindings_sku_not_blank") {
		t.Fatalf("SKU 不能为空白: %v", err)
	}
	if err := exec(t, pool, insertBinding, int64(3), "TB", "s3", int64(3)); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "UPDATE cnx.platform_sku_bindings SET last_stock = -1 WHERE id = 3"); !dbx.IsCheckViolation(err, "ck_bindings_last_stock") {
		t.Fatalf("同步库存不能为负: %v", err)
	}
	if err := exec(t, pool, "UPDATE cnx.platform_sku_bindings SET last_sync_status = 'WEIRD' WHERE id = 3"); !dbx.IsCheckViolation(err, "ck_bindings_sync_status") {
		t.Fatalf("同步状态受限: %v", err)
	}
	for _, st := range []string{"NEW", "SKIP", "SUCCESS", "FAIL"} {
		if err := exec(t, pool, "UPDATE cnx.platform_sku_bindings SET last_sync_status = $1, last_sync_at = now() WHERE id = 3", st); err != nil {
			t.Fatalf("状态 %s 应允许: %v", st, err)
		}
	}
	var enabled bool
	if err := pool.QueryRow(context.Background(), "SELECT sync_enabled FROM cnx.platform_sku_bindings WHERE id = 3").Scan(&enabled); err != nil || !enabled {
		t.Fatalf("默认开启同步: %v %v", enabled, err)
	}
}

// 同步任务按“最久未同步优先”取批：从未同步的排最前，其后按 last_sync_at 升序，且被停用同步的不参与。
func TestSyncRotationOrderingAndIndexUse(t *testing.T) {
	pool := migrated(t)
	rows := []struct {
		id      int64
		syncAt  string
		enabled bool
	}{
		{1, "2030-01-03T00:00:00Z", true},
		{2, "", true}, // 从未同步
		{3, "2030-01-01T00:00:00Z", true},
		{4, "2029-01-01T00:00:00Z", false}, // 停用，不应出现
		{5, "2030-01-02T00:00:00Z", true},
	}
	for _, r := range rows {
		if err := exec(t, pool, insertBinding, r.id, "MOCK", "sku-"+string(rune('0'+r.id)), r.id+1000); err != nil {
			t.Fatal(err)
		}
		if r.syncAt != "" {
			if err := exec(t, pool, "UPDATE cnx.platform_sku_bindings SET last_sync_at = $2::timestamptz WHERE id = $1", r.id, r.syncAt); err != nil {
				t.Fatal(err)
			}
		}
		if !r.enabled {
			if err := exec(t, pool, "UPDATE cnx.platform_sku_bindings SET sync_enabled = false WHERE id = $1", r.id); err != nil {
				t.Fatal(err)
			}
		}
	}

	const query = "SELECT id FROM cnx.platform_sku_bindings WHERE sync_enabled ORDER BY last_sync_at NULLS FIRST, id LIMIT 10"
	got, err := pgx.CollectRows(mustQuery(t, pool, query), pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{2, 3, 5, 1}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	// 强制禁用顺序扫描，证明该排序能被部分索引 idx_bindings_sync_rotation 直接满足
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	planRows, err := tx.Query(context.Background(), "EXPLAIN (COSTS OFF) "+query)
	if err != nil {
		t.Fatal(err)
	}
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	planRows.Close()
	if !strings.Contains(plan.String(), "idx_bindings_sync_rotation") || strings.Contains(plan.String(), "Sort") {
		t.Fatalf("应走 idx_bindings_sync_rotation 且无需额外排序:\n%s", plan.String())
	}
}

func mustQuery(t *testing.T, pool *pgxpool.Pool, sql string) pgx.Rows {
	t.Helper()
	rows, err := pool.Query(context.Background(), sql)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

const insertPriceMap = `INSERT INTO cnx.price_maps (id, coupon_id, platform, external_sku_id, manual_price, currency) VALUES ($1, $2, $3, 'sku', $4, $5)`

func TestPriceMapConstraints(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, insertPriceMap, int64(1), int64(1001), "JD", 19.9, "CNY"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, insertPriceMap, int64(2), int64(1001), "JD", 20.0, "CNY"); !dbx.IsUniqueViolation(err, "uk_price_maps_coupon_platform") {
		t.Fatalf("每张券每个平台至多一条映射: %v", err)
	}
	if err := exec(t, pool, insertPriceMap, int64(3), int64(1001), "TB", 20.0, "CNY"); err != nil {
		t.Fatalf("同一张券可映射多个平台: %v", err)
	}
	if err := exec(t, pool, insertPriceMap, int64(4), int64(1002), "JD", -1.0, "CNY"); !dbx.IsCheckViolation(err, "ck_price_maps_price") {
		t.Fatalf("手工价不能为负: %v", err)
	}
	if err := exec(t, pool, insertPriceMap, int64(5), int64(1003), "JD", 1.0, "yuan"); !dbx.IsCheckViolation(err, "ck_price_maps_currency") {
		t.Fatalf("币种必须是 3 位大写: %v", err)
	}
	if err := exec(t, pool, insertPriceMap, int64(6), int64(1004), "JD", nil, "CNY"); err != nil {
		t.Fatalf("手工价可为空: %v", err)
	}
	if err := exec(t, pool, "UPDATE cnx.price_maps SET last_sync_status = 'FAIL', last_sync_at = now() WHERE id = 6"); err != nil {
		t.Fatalf("price_maps 应有同步进度列: %v", err)
	}
}
