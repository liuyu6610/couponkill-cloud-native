package migrations_test

import (
	"context"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

const schema = "cpn"

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

// addCoupon 插入一张最小合法的券。
func addCoupon(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	if err := exec(t, pool, "INSERT INTO cpn.coupons (id, name, type, total_stock) VALUES ($1, 'c', 2, 100)", id); err != nil {
		t.Fatalf("addCoupon: %v", err)
	}
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
	if n := count(t, pool, "SELECT count(*) FROM cpn.schema_migrations"); n != len(files) {
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

func TestNegativeStockIsRejectedByCheck(t *testing.T) {
	pool := migrated(t)
	addCoupon(t, pool, 1)

	err := exec(t, pool, "INSERT INTO cpn.coupon_stock (coupon_id, bucket, remaining) VALUES (1, 0, -1)")
	if !dbx.IsCheckViolation(err, "ck_coupon_stock_remaining") {
		t.Fatalf("负库存应被 CHECK 拒绝: %v", err)
	}
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock (coupon_id, bucket, remaining) VALUES (1, 0, 1)"); err != nil {
		t.Fatal(err)
	}
	// 绕过 WHERE remaining > 0 的裸扣减会把库存减成 -1，必须被拒绝
	if err := exec(t, pool, "UPDATE cpn.coupon_stock SET remaining = remaining - 2 WHERE coupon_id = 1 AND bucket = 0"); !dbx.IsCheckViolation(err, "ck_coupon_stock_remaining") {
		t.Fatalf("超扣应被 CHECK 拒绝: %v", err)
	}
}

func TestBucketDeductNeverGoesBelowZeroUnderConcurrency(t *testing.T) {
	pool := migrated(t)
	addCoupon(t, pool, 1)
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock (coupon_id, bucket, remaining) VALUES (1, 0, 50)"); err != nil {
		t.Fatal(err)
	}

	const workers, attempts = 8, 20 // 160 次尝试抢 50 个库存
	var won atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < attempts; i++ {
				tag, err := pool.Exec(context.Background(),
					"UPDATE cpn.coupon_stock SET remaining = remaining - 1 WHERE coupon_id = 1 AND bucket = 0 AND remaining > 0")
				if err != nil {
					t.Errorf("扣减不应报错(售罄只是 0 行受影响): %v", err)
					return
				}
				won.Add(tag.RowsAffected())
			}
		}()
	}
	wg.Wait()
	if won.Load() != 50 {
		t.Fatalf("恰好 50 次应成功, got %d", won.Load())
	}
	if n := count(t, pool, "SELECT remaining FROM cpn.coupon_stock WHERE coupon_id = 1 AND bucket = 0"); n != 0 {
		t.Fatalf("最终库存应为 0, got %d", n)
	}
}

func TestStockRowsFollowCouponDeletion(t *testing.T) {
	pool := migrated(t)
	addCoupon(t, pool, 1)
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (1, 0, 5), (1, 1, 5)"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (999, 0, 5)"); !dbx.IsForeignKeyViolation(err, "fk_coupon_stock_coupon") {
		t.Fatalf("不存在的券不能有库存桶: %v", err)
	}
	if err := exec(t, pool, "DELETE FROM cpn.coupons WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "SELECT count(*) FROM cpn.coupon_stock"); n != 0 {
		t.Fatalf("删券应级联删除库存桶, got %d", n)
	}
}

func TestStockBucketPrimaryKey(t *testing.T) {
	pool := migrated(t)
	addCoupon(t, pool, 1)
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (1, 0, 5)"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (1, 0, 5)"); !dbx.IsUniqueViolation(err, "pk_coupon_stock") {
		t.Fatalf("同一 (coupon_id, bucket) 不能重复: %v", err)
	}
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (1, -1, 5)"); !dbx.IsCheckViolation(err, "ck_coupon_stock_bucket") {
		t.Fatalf("桶号不能为负: %v", err)
	}
}

func TestLedgerPrimaryKeyMakesDeductionIdempotent(t *testing.T) {
	pool := migrated(t)
	const ins = `INSERT INTO cpn.stock_ledger (request_id, coupon_id, bucket, user_id) VALUES ($1, 1, 3, 42)`

	if err := exec(t, pool, ins, "req-1"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, ins, "req-1"); !dbx.IsUniqueViolation(err, "pk_stock_ledger") {
		t.Fatalf("同一 request_id 不能重复入账: %v", err)
	}
	tag, err := pool.Exec(context.Background(), ins+" ON CONFLICT (request_id) DO NOTHING", "req-1")
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("ON CONFLICT DO NOTHING 应返回 0 行: %v %v", tag, err)
	}
	var state string
	if err := pool.QueryRow(context.Background(), "SELECT state FROM cpn.stock_ledger WHERE request_id = 'req-1'").Scan(&state); err != nil || state != "deducted" {
		t.Fatalf("新流水默认 deducted: %q %v", state, err)
	}
	if err := exec(t, pool, "UPDATE cpn.stock_ledger SET state = 'released' WHERE request_id = 'req-1' AND state = 'deducted'"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "UPDATE cpn.stock_ledger SET state = 'bogus' WHERE request_id = 'req-1'"); !dbx.IsCheckViolation(err, "ck_stock_ledger_state") {
		t.Fatalf("非法状态应被拒绝: %v", err)
	}
}

func TestDeductAndLedgerInOneTransactionRollBackTogether(t *testing.T) {
	pool := migrated(t)
	addCoupon(t, pool, 1)
	if err := exec(t, pool, "INSERT INTO cpn.coupon_stock VALUES (1, 0, 5)"); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), "UPDATE cpn.coupon_stock SET remaining = remaining - 1 WHERE coupon_id = 1 AND bucket = 0 AND remaining > 0"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), "INSERT INTO cpn.stock_ledger (request_id, coupon_id, bucket, user_id) VALUES ('r', 1, 0, 9)"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "SELECT remaining FROM cpn.coupon_stock WHERE coupon_id = 1"); n != 5 {
		t.Fatalf("回滚后库存应保持 5, got %d", n)
	}
	if n := count(t, pool, "SELECT count(*) FROM cpn.stock_ledger"); n != 0 {
		t.Fatalf("回滚后不应有流水, got %d", n)
	}
}

func TestCouponChecks(t *testing.T) {
	pool := migrated(t)
	const ins = `INSERT INTO cpn.coupons (id, name, type, total_stock, valid_days, per_user_limit, face_value, seckill_start_at, seckill_end_at, status)
		VALUES ($1, 'c', $2, $3, $4, $5, $6, $7::timestamptz, $8::timestamptz, $9)`
	cases := []struct {
		name       string
		check      string
		typ        int
		stock      int
		days       int
		limit      int
		face       float64
		start, end any
		status     int
	}{
		{"非法类型", "ck_coupons_type", 3, 1, 1, 1, 1, nil, nil, 1},
		{"负总库存", "ck_coupons_total_stock", 2, -1, 1, 1, 1, nil, nil, 1},
		{"有效期必须为正", "ck_coupons_valid_days", 2, 1, 0, 1, 1, nil, nil, 1},
		{"每人限领必须为正", "ck_coupons_per_user_limit", 2, 1, 1, 0, 1, nil, nil, 1},
		{"面额不能为负", "ck_coupons_face_value", 2, 1, 1, 1, -1, nil, nil, 1},
		{"非法状态", "ck_coupons_status", 2, 1, 1, 1, 1, nil, nil, 5},
		{"时间窗只设开始", "ck_coupons_seckill_window", 2, 1, 1, 1, 1, "2030-01-01T00:00:00Z", nil, 1},
		{"时间窗只设结束", "ck_coupons_seckill_window", 2, 1, 1, 1, 1, nil, "2030-01-01T00:00:00Z", 1},
		{"结束不晚于开始", "ck_coupons_seckill_window", 2, 1, 1, 1, 1, "2030-01-02T00:00:00Z", "2030-01-01T00:00:00Z", 1},
	}
	for i, c := range cases {
		err := exec(t, pool, ins, int64(100+i), c.typ, c.stock, c.days, c.limit, c.face, c.start, c.end, c.status)
		if !dbx.IsCheckViolation(err, c.check) {
			t.Errorf("%s: 期望违反 %s, got %v", c.name, c.check, err)
		}
	}
	if err := exec(t, pool, ins, int64(1), 2, 10, 7, 1, 50.0, "2030-01-01T00:00:00Z", "2030-01-02T00:00:00Z", 1); err != nil {
		t.Fatalf("合法的带时间窗的秒杀券: %v", err)
	}
	if err := exec(t, pool, ins, int64(2), 1, 10, 7, 1, 10.0, nil, nil, 1); err != nil {
		t.Fatalf("合法的无时间窗常驻券: %v", err)
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
	if n := count(t, pool, "SELECT count(*) FROM cpn.coupons"); n != 0 {
		t.Fatalf("迁移不得写入演示券, got %d", n)
	}
}

func TestSeedDemoCreatesCouponsWithBucketsAndIsIdempotent(t *testing.T) {
	pool := migrated(t)
	seed := readSeed(t)
	for i := 0; i < 2; i++ {
		if err := exec(t, pool, seed); err != nil {
			t.Fatalf("第 %d 次执行种子: %v", i+1, err)
		}
	}
	if n := count(t, pool, "SELECT count(*) FROM cpn.coupons"); n != 2 {
		t.Fatalf("应恰好 2 张演示券, got %d", n)
	}
	if n := count(t, pool, "SELECT type FROM cpn.coupons WHERE id = 1001"); n != 2 {
		t.Fatalf("1001 应是秒杀券, type=%d", n)
	}
	if n := count(t, pool, "SELECT type FROM cpn.coupons WHERE id = 1002"); n != 1 {
		t.Fatalf("1002 应是常驻券, type=%d", n)
	}
	for _, id := range []int64{1001, 1002} {
		if n := count(t, pool, "SELECT count(*) FROM cpn.coupon_stock WHERE coupon_id = $1", id); n != 8 {
			t.Fatalf("券 %d 应有 8 个库存桶, got %d", id, n)
		}
		if n := count(t, pool, "SELECT sum(remaining)::int FROM cpn.coupon_stock WHERE coupon_id = $1", id); n != 1600 {
			t.Fatalf("券 %d 的库存桶之和应为 1600(=total_stock), got %d", id, n)
		}
		if n := count(t, pool, "SELECT total_stock FROM cpn.coupons WHERE id = $1", id); n != 1600 {
			t.Fatalf("券 %d total_stock 应为 1600, got %d", id, n)
		}
	}
}

func TestSeedDoesNotResetConsumedStock(t *testing.T) {
	pool := migrated(t)
	seed := readSeed(t)
	if err := exec(t, pool, seed); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "UPDATE cpn.coupon_stock SET remaining = 0 WHERE coupon_id = 1001 AND bucket = 0"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, seed); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, "SELECT remaining FROM cpn.coupon_stock WHERE coupon_id = 1001 AND bucket = 0"); n != 0 {
		t.Fatalf("重复执行种子不得把已消耗的库存复位, got %d", n)
	}
}
