package migrations_test

import (
	"context"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

const schema = "ord"

// migrated 返回一个已应用 ord 迁移的隔离数据库。
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

// insertOrder 插入一笔订单；expire 固定在未来，status/cancel/use 由调用方决定。
func insertOrder(pool *pgxpool.Pool, id, userID, couponID int64, requestID string, status int, extra string) error {
	cols := "id, user_id, coupon_id, request_id, coupon_name, coupon_type, expire_time, status"
	vals := "$1, $2, $3, $4, 'c', 2, now() + interval '7 days', $5"
	if extra != "" {
		parts := strings.SplitN(extra, "=", 2)
		cols += ", " + parts[0]
		vals += ", " + parts[1]
	}
	_, err := pool.Exec(context.Background(),
		"INSERT INTO ord.orders ("+cols+") VALUES ("+vals+")", id, userID, couponID, requestID, status)
	return err
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
	if n := count(t, pool, "SELECT count(*) FROM ord.schema_migrations"); n != len(files) {
		t.Fatalf("版本记录数 %d != 文件数 %d", n, len(files))
	}
	for _, table := range []string{"orders", "reservations", "notifications"} {
		if n := count(t, pool, "SELECT count(*) FROM pg_tables WHERE schemaname='ord' AND tablename=$1", table); n != 1 {
			t.Fatalf("缺少表 ord.%s", table)
		}
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

func TestOrdersAreHashPartitionedIntoSixteen(t *testing.T) {
	pool := migrated(t)
	if n := count(t, pool, `SELECT count(*) FROM pg_inherits WHERE inhparent = 'ord.orders'::regclass`); n != 16 {
		t.Fatalf("应有 16 个分区, got %d", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM pg_partitioned_table WHERE partrelid = 'ord.orders'::regclass AND partstrat = 'h'`); n != 1 {
		t.Fatal("orders 应按 HASH 分区")
	}

	// 200 个不同用户的订单应落到全部 16 个分区，而不是挤在少数几个
	for u := int64(1); u <= 200; u++ {
		if err := insertOrder(pool, u, u, 1, "r", 1, ""); err != nil {
			t.Fatalf("insert user %d: %v", u, err)
		}
	}
	if n := count(t, pool, `SELECT count(DISTINCT tableoid) FROM ord.orders`); n != 16 {
		t.Fatalf("订单应分布在全部 16 个分区, got %d", n)
	}
}

func explain(t *testing.T, pool *pgxpool.Pool, sql string) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN (COSTS OFF) "+sql)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

var partitionNameRE = regexp.MustCompile(`\borders_p[0-9]{2}\b`)

// scannedPartitions 返回执行计划里出现的不同分区个数。
func scannedPartitions(plan string) int {
	seen := map[string]bool{}
	for _, m := range partitionNameRE.FindAllString(plan, -1) {
		seen[m] = true
	}
	return len(seen)
}

func TestPartitionPruningOnUserID(t *testing.T) {
	pool := migrated(t)

	pruned := explain(t, pool, "SELECT * FROM ord.orders WHERE user_id = 12345")
	if n := scannedPartitions(pruned); n != 1 {
		t.Fatalf("WHERE user_id = $1 应只扫描 1 个分区, got %d:\n%s", n, pruned)
	}

	unpruned := explain(t, pool, "SELECT * FROM ord.orders WHERE coupon_id = 7")
	if n := scannedPartitions(unpruned); n < 16 {
		t.Fatalf("不带 user_id 的查询应扫描全部分区(证明上面的裁剪是真实的), got %d:\n%s", n, unpruned)
	}
}

func TestActiveOrderUniquePerUserAndCoupon(t *testing.T) {
	pool := migrated(t)
	const user, coupon = 42, 1001

	if err := insertOrder(pool, 1, user, coupon, "req-1", 1, ""); err != nil {
		t.Fatal(err)
	}
	err := insertOrder(pool, 2, user, coupon, "req-2", 1, "")
	if !dbx.IsUniqueViolation(err, "") {
		t.Fatalf("同一用户同一券的第二笔未取消订单应违反部分唯一索引, got %v", err)
	}
	// 其他用户、其他券不受影响
	if err := insertOrder(pool, 3, user+1, coupon, "req-3", 1, ""); err != nil {
		t.Fatalf("其他用户应可下单: %v", err)
	}
	if err := insertOrder(pool, 4, user, coupon+1, "req-4", 1, ""); err != nil {
		t.Fatalf("同一用户别的券应可下单: %v", err)
	}
}

func TestCancelledOrderDoesNotBlockNewActiveOrder(t *testing.T) {
	pool := migrated(t)
	const user, coupon = 42, 1001

	if err := insertOrder(pool, 1, user, coupon, "req-1", 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, "UPDATE ord.orders SET status = 4, cancel_time = now() WHERE user_id = $1 AND id = 1", user); err != nil {
		t.Fatalf("取消: %v", err)
	}
	if err := insertOrder(pool, 2, user, coupon, "req-2", 1, ""); err != nil {
		t.Fatalf("取消后应能再抢: %v", err)
	}
	// 再取消、再抢，已取消订单可以有多笔
	if err := exec(t, pool, "UPDATE ord.orders SET status = 4, cancel_time = now() WHERE user_id = $1 AND id = 2", user); err != nil {
		t.Fatal(err)
	}
	if err := insertOrder(pool, 3, user, coupon, "req-3", 1, ""); err != nil {
		t.Fatalf("多笔已取消订单不应互相阻塞: %v", err)
	}
	if n := count(t, pool, "SELECT count(*) FROM ord.orders WHERE user_id = $1 AND coupon_id = $2", user, coupon); n != 3 {
		t.Fatalf("got %d", n)
	}
	// 复活一笔已取消订单（status 改回 1）会与当前活跃订单冲突
	err := exec(t, pool, "UPDATE ord.orders SET status = 1, cancel_time = NULL WHERE user_id = $1 AND id = 1", user)
	if !dbx.IsUniqueViolation(err, "") {
		t.Fatalf("复活已取消订单应违反部分唯一索引, got %v", err)
	}
}

func TestRequestIDUniquePerUser(t *testing.T) {
	pool := migrated(t)

	if err := insertOrder(pool, 1, 42, 1001, "req-1", 1, ""); err != nil {
		t.Fatal(err)
	}
	// 同一用户重复的 request_id（即使券不同）违反 UNIQUE(user_id, request_id)
	err := insertOrder(pool, 2, 42, 1002, "req-1", 1, "")
	if !dbx.IsUniqueViolation(err, "") {
		t.Fatalf("同一用户重复 request_id 应被拒绝, got %v", err)
	}
	// 不同用户可以用同一个 request_id
	if err := insertOrder(pool, 3, 43, 1001, "req-1", 1, ""); err != nil {
		t.Fatalf("不同用户的 request_id 互不影响: %v", err)
	}
}

// 分区表上的唯一冲突报告的是分区自己的索引名；服务若要区分是哪条规则触发，只能按这个稳定的命名模式匹配。
func TestUniqueViolationNamesFollowStablePattern(t *testing.T) {
	pool := migrated(t)
	if err := insertOrder(pool, 1, 42, 1001, "req-1", 1, ""); err != nil {
		t.Fatal(err)
	}
	var pe *pgconn.PgError

	byCoupon := insertOrder(pool, 2, 42, 1001, "req-2", 1, "")
	if !errors.As(byCoupon, &pe) || !regexp.MustCompile(`^orders_p\d{2}_user_id_coupon_id_idx$`).MatchString(pe.ConstraintName) {
		t.Fatalf("部分唯一索引的冲突名不符合约定: %v", byCoupon)
	}
	byRequest := insertOrder(pool, 3, 42, 1002, "req-1", 1, "")
	if !errors.As(byRequest, &pe) || !regexp.MustCompile(`^orders_p\d{2}_user_id_request_id_key$`).MatchString(pe.ConstraintName) {
		t.Fatalf("(user_id, request_id) 的冲突名不符合约定: %v", byRequest)
	}
}

func TestOrderInsertOnConflictMakesReplayIdempotent(t *testing.T) {
	pool := migrated(t)
	const insert = `INSERT INTO ord.orders (id, user_id, coupon_id, request_id, coupon_name, coupon_type, expire_time)
		VALUES ($1, 42, 1001, 'req-1', 'c', 2, now() + interval '1 day')
		ON CONFLICT (user_id, request_id) DO NOTHING`

	first, err := pool.Exec(context.Background(), insert, int64(1))
	if err != nil || first.RowsAffected() != 1 {
		t.Fatalf("首次应插入 1 行: %v %v", first, err)
	}
	replay, err := pool.Exec(context.Background(), insert, int64(2))
	if err != nil || replay.RowsAffected() != 0 {
		t.Fatalf("重放应静默跳过: %v %v", replay, err)
	}
}

func TestOrderCheckConstraints(t *testing.T) {
	pool := migrated(t)

	cases := []struct {
		name  string
		check string
		err   error
	}{
		{"已取消订单必须有 cancel_time", "ck_orders_cancelled_has_time", insertOrder(pool, 1, 1, 1, "a", 4, "")},
		{"已使用订单必须有 use_time", "ck_orders_used_has_time", insertOrder(pool, 2, 1, 1, "b", 2, "")},
		{"状态只能是 1..4", "ck_orders_status", insertOrder(pool, 3, 1, 1, "c", 9, "")},
		{"来源受限", "ck_orders_source", insertOrder(pool, 4, 1, 1, "d", 1, "source='hack'")},
		{"过期时间必须晚于领取时间", "ck_orders_expire_after_get", insertOrder(pool, 5, 1, 1, "e", 1, "get_time=now() + interval '30 days'")},
	}
	for _, c := range cases {
		if !dbx.IsCheckViolation(c.err, "") || !strings.Contains(c.err.Error(), c.check) {
			t.Errorf("%s: 期望违反 %s, got %v", c.name, c.check, c.err)
		}
	}
	// 合法的已取消 / 已使用订单可以写入
	if err := insertOrder(pool, 10, 2, 1, "ok1", 4, "cancel_time=now()"); err != nil {
		t.Fatalf("合法已取消订单: %v", err)
	}
	if err := insertOrder(pool, 11, 3, 1, "ok2", 2, "use_time=now()"); err != nil {
		t.Fatalf("合法已使用订单: %v", err)
	}
}

func TestReservationActiveUniquePerUserAndCoupon(t *testing.T) {
	pool := migrated(t)
	const ins = `INSERT INTO ord.reservations (id, user_id, coupon_id, status, trigger_at)
		VALUES ($1, 42, 1001, $2, now() + interval '1 hour')`

	if err := exec(t, pool, ins, int64(1), "PENDING"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, ins, int64(2), "PENDING"); !dbx.IsUniqueViolation(err, "uk_reservations_active_user_coupon") {
		t.Fatalf("同一用户同一券的第二个进行中预约应被拒绝, got %v", err)
	}
	for id, st := range map[int64]string{3: "FIRING", 4: "QUEUED"} {
		if err := exec(t, pool, ins, id, st); !dbx.IsUniqueViolation(err, "uk_reservations_active_user_coupon") {
			t.Fatalf("%s 也属于进行中状态: %v", st, err)
		}
	}
	if err := exec(t, pool, "UPDATE ord.reservations SET status = 'CANCELLED' WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, pool, ins, int64(5), "PENDING"); err != nil {
		t.Fatalf("取消后应能重新预约: %v", err)
	}
	// 终态（SUCCESS/FAILED/EXPIRED/CANCELLED）可以有多条
	for id, st := range map[int64]string{6: "SUCCESS", 7: "FAILED", 8: "EXPIRED", 9: "CANCELLED"} {
		if err := exec(t, pool, ins, id, st); err != nil {
			t.Fatalf("终态 %s 不应受限: %v", st, err)
		}
	}
}

func TestReservationChecks(t *testing.T) {
	pool := migrated(t)

	err := exec(t, pool, "INSERT INTO ord.reservations (id, user_id, coupon_id, status) VALUES (1, 1, 1, 'BOGUS')")
	if !dbx.IsCheckViolation(err, "ck_reservations_status") {
		t.Fatalf("非法状态应被拒绝: %v", err)
	}
	err = exec(t, pool, "INSERT INTO ord.reservations (id, user_id, coupon_id) VALUES (2, 1, 1)")
	if !dbx.IsCheckViolation(err, "ck_reservations_pending_has_trigger") {
		t.Fatalf("PENDING 必须有 trigger_at: %v", err)
	}
	err = exec(t, pool, "INSERT INTO ord.reservations (id, user_id, coupon_id, trigger_at, retry_count) VALUES (3, 1, 1, now(), -1)")
	if !dbx.IsCheckViolation(err, "ck_reservations_retry_count") {
		t.Fatalf("retry_count 不能为负: %v", err)
	}
}

func TestNotificationDefaults(t *testing.T) {
	pool := migrated(t)
	if err := exec(t, pool, "INSERT INTO ord.notifications (id, user_id, type, title) VALUES (1, 42, 'RESERVATION_SUCCESS', 'hi')"); err != nil {
		t.Fatal(err)
	}
	var read bool
	if err := pool.QueryRow(context.Background(), "SELECT read_flag FROM ord.notifications WHERE id = 1").Scan(&read); err != nil || read {
		t.Fatalf("新通知默认未读: read=%v err=%v", read, err)
	}
	if err := exec(t, pool, "INSERT INTO ord.notifications (id, user_id, type) VALUES (2, 42, 'x')"); err == nil {
		t.Fatal("title 必填")
	}
}
