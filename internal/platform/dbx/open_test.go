package dbx_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

// waitFor 每 50ms 轮询 cond，最多 10 秒。
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// syncBuffer 是并发安全的 bytes.Buffer，用来收集 slog 输出。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func openWith(t *testing.T, opts dbx.Options) *pgxpool.Pool {
	t.Helper()
	pool, err := dbx.Open(context.Background(), dbxtest.NewDSN(t), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestOpenAppliesPoolSettings(t *testing.T) {
	pool := openWith(t, dbx.Options{MaxConns: 7, MinConns: 1, ApplicationName: "dbx-open-test", StatementTimeout: 4 * time.Second})

	if got := pool.Config().MaxConns; got != 7 {
		t.Fatalf("MaxConns=%d", got)
	}
	var app, timeout string
	if err := pool.QueryRow(context.Background(), "SELECT current_setting('application_name'), current_setting('statement_timeout')").Scan(&app, &timeout); err != nil {
		t.Fatal(err)
	}
	if app != "dbx-open-test" || timeout != "4s" {
		t.Fatalf("application_name=%q statement_timeout=%q", app, timeout)
	}
}

func TestOpenDefaults(t *testing.T) {
	pool := openWith(t, dbx.Options{})
	cfg := pool.Config()
	if cfg.MaxConns != 20 || cfg.MinConns != 2 {
		t.Fatalf("默认连接数不对: max=%d min=%d", cfg.MaxConns, cfg.MinConns)
	}
	if cfg.HealthCheckPeriod != 30*time.Second || cfg.MaxConnLifetime != 30*time.Minute {
		t.Fatalf("默认健康检查/生命周期不对: %v %v", cfg.HealthCheckPeriod, cfg.MaxConnLifetime)
	}
	var timeout string
	if err := pool.QueryRow(context.Background(), "SHOW statement_timeout").Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if timeout != "30s" {
		t.Fatalf("默认 statement_timeout 应为 30s, got %q", timeout)
	}
}

func TestOpenFailsFastOnUnreachableServer(t *testing.T) {
	dbxtest.NewDSN(t) // 仅用于在 PG 不可用时跳过
	start := time.Now()
	_, err := dbx.Open(context.Background(), "postgres://nobody:x@127.0.0.1:1/none?sslmode=disable", dbx.Options{ConnectTimeout: time.Second})
	if err == nil {
		t.Fatal("连不上应返回错误")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("应在 ConnectTimeout 附近失败, 实际 %v", time.Since(start))
	}
}

func TestOpenRejectsMalformedDSN(t *testing.T) {
	if _, err := dbx.Open(context.Background(), "postgres://%zz", dbx.Options{}); err == nil {
		t.Fatal("非法 DSN 应报错")
	}
}

func TestSlowQueriesAreLoggedWithoutArguments(t *testing.T) {
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	pool := openWith(t, dbx.Options{SlowQueryThreshold: 20 * time.Millisecond, Logger: logger})

	if _, err := pool.Exec(context.Background(), "SELECT pg_sleep(0.1), $1::text", "s3cret-password"); err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	if !strings.Contains(out, "慢查询") || !strings.Contains(out, "pg_sleep") {
		t.Fatalf("慢查询应被记录: %s", out)
	}
	if strings.Contains(out, "s3cret-password") {
		t.Fatal("慢查询日志不得包含绑定参数")
	}
}

func TestFastQueriesAreNotLogged(t *testing.T) {
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	pool := openWith(t, dbx.Options{SlowQueryThreshold: 5 * time.Second, Logger: logger})

	if _, err := pool.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "慢查询") {
		t.Fatalf("快查询不应出现在慢查询日志里: %s", logs.String())
	}
}

func TestReadyReportsHealthAndFailure(t *testing.T) {
	pool := openWith(t, dbx.Options{})
	ready := dbx.Ready(pool)
	if err := ready(context.Background()); err != nil {
		t.Fatalf("健康时应就绪: %v", err)
	}
	pool.Close()
	if err := ready(context.Background()); err == nil {
		t.Fatal("连接池关闭后应报告未就绪")
	}
}

func TestWithTxCommitAndRollbackAgainstRealPostgres(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE TABLE kv (k TEXT PRIMARY KEY, v INT NOT NULL)"); err != nil {
		t.Fatal(err)
	}

	if err := dbx.WithTx(ctx, pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO kv VALUES ('a', 1)")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err := dbx.WithTx(ctx, pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO kv VALUES ('b', 2)"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM kv").Scan(&n); err != nil || n != 1 {
		t.Fatalf("回滚后只应剩 1 行: n=%d err=%v", n, err)
	}
}

func TestWithTxPanicReleasesConnectionWithoutOpenTransaction(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE TABLE kv (k TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}

	func() {
		defer func() { _ = recover() }()
		_ = dbx.WithTx(ctx, pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "INSERT INTO kv VALUES ('x')"); err != nil {
				return err
			}
			panic("kaboom")
		})
	}()

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM kv").Scan(&n); err != nil || n != 0 {
		t.Fatalf("panic 后事务必须已回滚: n=%d err=%v", n, err)
	}
	var idle int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state = 'idle in transaction'").Scan(&idle); err != nil || idle != 0 {
		t.Fatalf("不应有 idle in transaction 的连接: idle=%d err=%v", idle, err)
	}
}

// 两个可串行化事务互相读写对方写的行（write skew），PostgreSQL 会让其中一个以 40001 失败；
// 开启重试后两个事务都应最终成功。
func TestWithTxRetriesRealSerializationFailure(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE doctors (name TEXT PRIMARY KEY, on_call BOOL NOT NULL);
		INSERT INTO doctors VALUES ('alice', true), ('bob', true);`); err != nil {
		t.Fatal(err)
	}

	var attempts [2]int
	var barrier sync.WaitGroup
	barrier.Add(2)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, who := range []string{"alice", "bob"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = dbx.WithTx(ctx, pool, dbx.TxOptions{Isolation: pgx.Serializable, MaxRetries: 5, RetryBackoff: 5 * time.Millisecond}, func(tx pgx.Tx) error {
				attempts[i]++
				var onCall int
				if err := tx.QueryRow(ctx, "SELECT count(*) FROM doctors WHERE on_call").Scan(&onCall); err != nil {
					return err
				}
				if attempts[i] == 1 {
					barrier.Done() // 两个事务都读完之后才写，保证第一轮必然冲突
					barrier.Wait()
				}
				if onCall < 2 {
					return nil // 重试时看到对方已下班，自己就不再下班
				}
				_, err := tx.Exec(ctx, "UPDATE doctors SET on_call = false WHERE name = $1", who)
				return err
			})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("事务 %d 重试后应成功: %v", i, err)
		}
	}
	if attempts[0]+attempts[1] < 3 {
		t.Fatalf("至少应有一个事务因 40001 被重试: attempts=%v", attempts)
	}
	var onCall int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM doctors WHERE on_call").Scan(&onCall); err != nil || onCall < 1 {
		t.Fatalf("可串行化应保证至少一人在岗: onCall=%d err=%v", onCall, err)
	}
}

func TestErrorClassificationAgainstRealConstraints(t *testing.T) {
	pool := dbxtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE parent (id INT PRIMARY KEY);
		CREATE TABLE child (id INT PRIMARY KEY, parent_id INT CONSTRAINT fk_child_parent REFERENCES parent(id),
			qty INT CONSTRAINT ck_child_qty CHECK (qty >= 0), tag TEXT CONSTRAINT uk_child_tag UNIQUE);
		INSERT INTO parent VALUES (1);
		INSERT INTO child VALUES (1, 1, 1, 't');`); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Exec(ctx, "INSERT INTO child VALUES (2, 1, 1, 't')")
	if !dbx.IsUniqueViolation(err, "uk_child_tag") || dbx.IsUniqueViolation(err, "child_pkey") {
		t.Fatalf("唯一冲突判定错误: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO child VALUES (3, 99, 1, 'x')")
	if !dbx.IsForeignKeyViolation(err, "fk_child_parent") {
		t.Fatalf("外键冲突判定错误: %v", err)
	}
	_, err = pool.Exec(ctx, "INSERT INTO child VALUES (4, 1, -1, 'y')")
	if !dbx.IsCheckViolation(err, "ck_child_qty") {
		t.Fatalf("CHECK 冲突判定错误: %v", err)
	}
	var x int
	err = pool.QueryRow(ctx, "SELECT id FROM child WHERE id = 404").Scan(&x)
	if !dbx.IsNoRows(err) {
		t.Fatalf("无结果判定错误: %v", err)
	}
}
