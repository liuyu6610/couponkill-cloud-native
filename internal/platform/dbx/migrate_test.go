package dbx_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

// 故意不用 IF NOT EXISTS：若迁移被重复执行，这些语句会直接报错，测试因此能抓到“非幂等”。
func twoStepFS() fstest.MapFS {
	return fstest.MapFS{
		"0001_create_items.sql": {Data: []byte("CREATE TABLE items (id BIGINT PRIMARY KEY, name TEXT NOT NULL);")},
		"0002_add_price.sql":    {Data: []byte("ALTER TABLE items ADD COLUMN price NUMERIC(12,2) NOT NULL DEFAULT 0;")},
		"seed_demo.sql":         {Data: []byte("INSERT INTO items (id, name) VALUES (1, 'demo');")},
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestMigrateAppliesInOrderAndRecordsVersions(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()

	if err := dbx.Migrate(ctx, pool, "svc", twoStepFS()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// 文件里的名字没带 schema 前缀，应落在 svc 里而不是 public。
	if n := countRows(t, pool, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'svc' AND table_name = 'items'`); n != 3 {
		t.Fatalf("svc.items 应有 3 列(id,name,price), got %d", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'items'`); n != 0 {
		t.Fatal("不应污染 public schema")
	}

	rows, err := pool.Query(ctx, "SELECT version, name FROM svc.schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var v int64
		var name string
		if err := rows.Scan(&v, &name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if len(got) != 2 || got[0] != "create_items" || got[1] != "add_price" {
		t.Fatalf("版本记录不对: %v", got)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM svc.items"); n != 0 {
		t.Fatal("seed_demo.sql 不是迁移，不应被执行")
	}
}

func TestMigrateTwiceIsNoOp(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := twoStepFS()

	if err := dbx.Migrate(ctx, pool, "svc", fsys); err != nil {
		t.Fatal(err)
	}
	var firstApplied string
	if err := pool.QueryRow(ctx, "SELECT max(applied_at)::text FROM svc.schema_migrations").Scan(&firstApplied); err != nil {
		t.Fatal(err)
	}

	if err := dbx.Migrate(ctx, pool, "svc", fsys); err != nil {
		t.Fatalf("第二次运行应是空操作而不是报错: %v", err)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM svc.schema_migrations"); n != 2 {
		t.Fatalf("版本记录应仍是 2 条, got %d", n)
	}
	var secondApplied string
	if err := pool.QueryRow(ctx, "SELECT max(applied_at)::text FROM svc.schema_migrations").Scan(&secondApplied); err != nil {
		t.Fatal(err)
	}
	if firstApplied != secondApplied {
		t.Fatal("第二次运行不应改写 applied_at")
	}
}

func TestMigrateAppliesOnlyNewFilesOnUpgrade(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := twoStepFS()
	delete(fsys, "0002_add_price.sql")

	if err := dbx.Migrate(ctx, pool, "svc", fsys); err != nil {
		t.Fatal(err)
	}
	if err := dbx.Migrate(ctx, pool, "svc", twoStepFS()); err != nil {
		t.Fatalf("升级时只应补跑新文件: %v", err)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM svc.schema_migrations"); n != 2 {
		t.Fatalf("got %d", n)
	}
}

func TestMigrateDetectsEditedAppliedFile(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	if err := dbx.Migrate(ctx, pool, "svc", twoStepFS()); err != nil {
		t.Fatal(err)
	}

	edited := twoStepFS()
	edited["0001_create_items.sql"] = &fstest.MapFile{
		Data: []byte("CREATE TABLE items (id BIGINT PRIMARY KEY, name TEXT NOT NULL, sneaky INT);"),
	}
	// 同时加一个新文件：校验和失败必须发生在应用任何新迁移之前。
	edited["0003_new.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE should_not_exist (id INT);")}

	err := dbx.Migrate(ctx, pool, "svc", edited)
	if !errors.Is(err, dbx.ErrChecksumMismatch) {
		t.Fatalf("应返回 ErrChecksumMismatch, got %v", err)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM information_schema.tables WHERE table_schema='svc' AND table_name='should_not_exist'"); n != 0 {
		t.Fatal("校验和不符时不应应用任何新迁移")
	}
}

func TestMigrateIgnoresLineEndingChangesInAppliedFile(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	lf := fstest.MapFS{"0001_t.sql": {Data: []byte("CREATE TABLE t (\n  id INT\n);\n")}}
	crlf := fstest.MapFS{"0001_t.sql": {Data: []byte("CREATE TABLE t (\r\n  id INT\r\n);\r\n")}}

	if err := dbx.Migrate(ctx, pool, "svc", lf); err != nil {
		t.Fatal(err)
	}
	if err := dbx.Migrate(ctx, pool, "svc", crlf); err != nil {
		t.Fatalf("仅换行风格不同不应判为被修改: %v", err)
	}
}

func TestMigrateFailureRollsBackThatFileOnly(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	broken := fstest.MapFS{
		"0001_ok.sql":   {Data: []byte("CREATE TABLE ok_table (id INT);")},
		"0002_bad.sql":  {Data: []byte("CREATE TABLE half_done (id INT);\nSELECT * FROM table_that_does_not_exist;")},
		"0003_next.sql": {Data: []byte("CREATE TABLE never_reached (id INT);")},
	}
	if err := dbx.Migrate(ctx, pool, "svc", broken); err == nil {
		t.Fatal("坏迁移必须让 Migrate 失败")
	}

	exists := func(table string) int {
		return countRows(t, pool, "SELECT count(*) FROM information_schema.tables WHERE table_schema='svc' AND table_name=$1", table)
	}
	if exists("ok_table") != 1 {
		t.Fatal("失败之前已成功的文件应保留")
	}
	if exists("half_done") != 0 {
		t.Fatal("失败的文件必须整体回滚（含它前半段的 DDL）")
	}
	if exists("never_reached") != 0 {
		t.Fatal("失败之后的文件不应执行")
	}
	if n := countRows(t, pool, "SELECT count(*) FROM svc.schema_migrations"); n != 1 {
		t.Fatalf("只应记录成功的版本, got %d", n)
	}

	// 修好 0002 后再次运行，应从失败处继续。
	fixed := fstest.MapFS{
		"0001_ok.sql":   broken["0001_ok.sql"],
		"0002_bad.sql":  {Data: []byte("CREATE TABLE half_done (id INT);")},
		"0003_next.sql": broken["0003_next.sql"],
	}
	// 0002 从未成功过，因此“修改”它是允许的。
	if err := dbx.Migrate(ctx, pool, "svc", fixed); err != nil {
		t.Fatalf("修复后应能继续: %v", err)
	}
	if exists("half_done") != 1 || exists("never_reached") != 1 {
		t.Fatal("修复后应补齐后续迁移")
	}
}

func TestMigrateConcurrentReplicasApplyExactlyOnce(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := twoStepFS() // 非幂等 DDL：任何一次重复执行都会报错

	const replicas = 8
	var wg sync.WaitGroup
	errs := make(chan error, replicas)
	for i := 0; i < replicas; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- dbx.Migrate(ctx, pool, "svc", fsys)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发启动的副本都应成功: %v", err)
		}
	}
	if n := countRows(t, pool, "SELECT count(*) FROM svc.schema_migrations"); n != 2 {
		t.Fatalf("每个版本应恰好记录一次, got %d", n)
	}
}

func TestMigrateSchemasAreIndependent(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"0001_t.sql": {Data: []byte("CREATE TABLE t (id INT);")}}

	if err := dbx.Migrate(ctx, pool, "aaa", fsys); err != nil {
		t.Fatal(err)
	}
	if err := dbx.Migrate(ctx, pool, "bbb", fsys); err != nil {
		t.Fatalf("同一份文件应能分别应用到另一个 schema: %v", err)
	}
}

func TestMigrateReleasesAdvisoryLock(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"0001_t.sql": {Data: []byte("CREATE TABLE t (id INT);")}}
	if err := dbx.Migrate(ctx, pool, "svc", fsys); err != nil {
		t.Fatal(err)
	}
	// pg_locks 是服务器级视图：并行运行的其他测试库里的迁移也会持有咨询锁，必须限定到当前库（咨询锁本就按库隔离）。
	if n := countRows(t, pool, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`); n != 0 {
		t.Fatalf("Migrate 返回后不应遗留咨询锁, got %d", n)
	}
}

func TestMigrateRejectsBadInput(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"0001_t.sql": {Data: []byte("SELECT 1;")}}

	if err := dbx.Migrate(ctx, pool, `x"; DROP SCHEMA public; --`, fsys); err == nil {
		t.Fatal("非法 schema 名应被拒绝")
	}
	dup := fstest.MapFS{
		"0001_a.sql": {Data: []byte("SELECT 1;")},
		"0001_b.sql": {Data: []byte("SELECT 1;")},
	}
	if err := dbx.Migrate(ctx, pool, "svc", dup); err == nil {
		t.Fatal("重复版本应被拒绝")
	}
}

func TestMigrateHonoursContextCancellationWhileWaitingForLock(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"0001_t.sql": {Data: []byte("CREATE TABLE t (id INT);")}}

	// 另一个会话持有同一把锁（键由包内 advisoryKey 派生，这里通过迁移期间的锁争用间接验证：
	// 先让一个迁移卡在长事务里持锁，再用带超时的 ctx 发起第二个迁移）。
	slow := fstest.MapFS{"0001_t.sql": {Data: []byte("SELECT pg_sleep(3); CREATE TABLE t (id INT);")}}
	done := make(chan error, 1)
	go func() { done <- dbx.Migrate(ctx, pool, "svc", slow) }()

	// 等第一个迁移真的在执行：active 状态且语句以迁移文件的内容开头。必须锚定开头——探测语句自己的文本里
	// 也含有 pg_sleep(3)，连接池里另一条空闲连接的「上一条语句」正是探测本身，用 LIKE '%...%' 会误判成
	// 「已经持锁」，第二个迁移于是抢先拿到锁，等锁超时的断言就会偶发落空。
	waitFor(t, func() bool {
		return countRows(t, pool, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'active' AND query LIKE 'SELECT pg_sleep(3);%' AND pid <> pg_backend_pid()") > 0
	})

	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	err := dbx.Migrate(short, pool, "svc", fsys)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等锁期间 ctx 超时应返回 DeadlineExceeded, got %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("持锁的迁移应正常完成: %v", err)
	}
}
