package repo_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// newRepo 创建一次性数据库、应用 usr 迁移并返回仓储与连接池。PostgreSQL 不可用时测试被跳过（或按 CK_REQUIRE_PG 失败）。
func newRepo(t *testing.T) (*repo.Repo, *pgxpool.Pool) {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "usr", migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return repo.New(pool), pool
}

func account(id int64, name string) service.NewAccount {
	return service.NewAccount{ID: id, Username: name, PasswordHash: "$2a$10$hash", Phone: "13800000000", Role: "user"}
}

func rowCount(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM usr.users WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestCreateAndFindRoundTrip(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	a := account(1, "alice")
	a.Email = "alice@example.com"

	created, err := r.Create(ctx, a)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != 1 || created.Username != "alice" || created.Phone != "13800000000" ||
		created.Email != "alice@example.com" || created.Role != "user" || created.Status != 1 ||
		created.LastActiveAt != nil || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}

	acc, err := r.FindByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if acc.PasswordHash != "$2a$10$hash" || acc.ID != 1 {
		t.Fatalf("acc = %+v", acc)
	}
	byID, err := r.FindByID(ctx, 1)
	if err != nil || byID.Username != "alice" {
		t.Fatalf("FindByID = %+v %v", byID, err)
	}
}

func TestEmptyPhoneAndEmailAreStoredAsNullAndReadBackEmpty(t *testing.T) {
	r, pool := newRepo(t)
	a := account(1, "bob")
	a.Phone = ""
	if _, err := r.Create(context.Background(), a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n := rowCount(t, pool, "username = 'bob' AND phone IS NULL AND email IS NULL"); n != 1 {
		t.Fatal("空串应存为 NULL")
	}
	u, err := r.FindByID(context.Background(), 1)
	if err != nil || u.Phone != "" || u.Email != "" {
		t.Fatalf("u = %+v %v", u, err)
	}
}

func TestFindNotFound(t *testing.T) {
	r, _ := newRepo(t)
	if _, err := r.FindByUsername(context.Background(), "ghost"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("FindByUsername err = %v", err)
	}
	if _, err := r.FindByID(context.Background(), 404); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("FindByID err = %v", err)
	}
}

func TestUsernameLookupIsExactAndCaseSensitive(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "Alice")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.FindByUsername(ctx, "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("用户名区分大小写，沿用 Java 语义: %v", err)
	}
	if _, err := r.Create(ctx, account(2, "alice")); err != nil {
		t.Fatalf("Alice 与 alice 是两个用户: %v", err)
	}
}

func TestDuplicateUsernameReturnsErrUsernameTaken(t *testing.T) {
	r, pool := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "alice")); err != nil {
		t.Fatal(err)
	}
	_, err := r.Create(ctx, account(2, "alice"))
	if !errors.Is(err, service.ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
	if n := rowCount(t, pool, "TRUE"); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

func TestPrimaryKeyConflictIsNotReportedAsUsernameTaken(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "alice")); err != nil {
		t.Fatal(err)
	}
	_, err := r.Create(ctx, account(1, "someone-else"))
	if err == nil || errors.Is(err, service.ErrUsernameTaken) {
		t.Fatalf("主键冲突不能被误报为用户名重复: %v", err)
	}
	if !dbx.IsUniqueViolation(err, "pk_users") {
		t.Fatalf("应保留原始 PG 错误以便排查: %v", err)
	}
}

func TestRoleCheckConstraintRejectsUnknownRole(t *testing.T) {
	r, _ := newRepo(t)
	a := account(1, "root")
	a.Role = "superuser"
	_, err := r.Create(context.Background(), a)
	if err == nil || !dbx.IsCheckViolation(err, "ck_users_role") {
		t.Fatalf("err = %v, want ck_users_role violation", err)
	}
	a.Role = "admin"
	if _, err := r.Create(context.Background(), a); err != nil {
		t.Fatalf("admin 角色应被接受: %v", err)
	}
}

func TestConcurrentRegistrationOfSameUsernameExactlyOneWins(t *testing.T) {
	r, pool := newRepo(t)
	const n = 20
	var wins, taken, other atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := r.Create(context.Background(), account(int64(100+i), "contested"))
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, service.ErrUsernameTaken):
				taken.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 || taken.Load() != n-1 || other.Load() != 0 {
		t.Fatalf("wins=%d taken=%d other=%d, want 1/%d/0", wins.Load(), taken.Load(), other.Load(), n-1)
	}
	if got := rowCount(t, pool, "username = 'contested'"); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}

func TestTouchLastActiveHonoursInterval(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "alice")); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)

	if ok, err := touch(r, ctx, 1, t0); err != nil || !ok {
		t.Fatalf("首次刷新应成功: %v %v", ok, err)
	}
	u, _ := r.FindByID(ctx, 1)
	if u.LastActiveAt == nil || !u.LastActiveAt.Equal(t0) {
		t.Fatalf("last_active_at = %v, want %v", u.LastActiveAt, t0)
	}
	if ok, err := touch(r, ctx, 1, t0.Add(59*time.Minute)); err != nil || ok {
		t.Fatalf("59 分钟内不应刷新: %v %v", ok, err)
	}
	if ok, err := touch(r, ctx, 1, t0.Add(time.Hour)); err != nil || !ok {
		t.Fatalf("恰好 1 小时应刷新: %v %v", ok, err)
	}
	if ok, err := touch(r, ctx, 999, t0); err != nil || ok {
		t.Fatalf("不存在的用户返回 false 而非报错: %v %v", ok, err)
	}
}

func TestTouchLastActiveConcurrentCallsUpdateOnce(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "alice")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	var updated atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := touch(r, ctx, 1, now)
			if err != nil {
				t.Errorf("Touch: %v", err)
			}
			if ok {
				updated.Add(1)
			}
		}()
	}
	wg.Wait()
	if updated.Load() != 1 {
		t.Fatalf("并发刷新同一用户应只有一次生效, got %d", updated.Load())
	}
}

func TestInsertBatchSkipsExistingUsernamesAndIsIdempotent(t *testing.T) {
	r, pool := newRepo(t)
	ctx := context.Background()
	if _, err := r.Create(ctx, account(1, "lt1")); err != nil {
		t.Fatal(err)
	}
	batch := []service.NewAccount{account(10, "lt0"), account(11, "lt1"), account(12, "lt2")}

	n, err := r.InsertBatch(ctx, batch)
	if err != nil || n != 2 {
		t.Fatalf("InsertBatch = %d %v, want 2", n, err)
	}
	n, err = r.InsertBatch(ctx, batch)
	if err != nil || n != 0 {
		t.Fatalf("重跑应插入 0 行: %d %v", n, err)
	}
	if got := rowCount(t, pool, "TRUE"); got != 3 {
		t.Fatalf("rows = %d, want 3", got)
	}
	if u, _ := r.FindByUsername(ctx, "lt1"); u.ID != 1 {
		t.Fatalf("已有用户不得被覆盖: %+v", u)
	}
}

func TestInsertBatchHandlesDuplicateUsernamesInsideOneBatchAndEmptyBatch(t *testing.T) {
	r, _ := newRepo(t)
	ctx := context.Background()
	n, err := r.InsertBatch(ctx, []service.NewAccount{account(1, "dup"), account(2, "dup")})
	if err != nil || n != 1 {
		t.Fatalf("同批内重复用户名只插入一次: %d %v", n, err)
	}
	if n, err := r.InsertBatch(ctx, nil); err != nil || n != 0 {
		t.Fatalf("空批: %d %v", n, err)
	}
}

func TestSeedDemoIsIdempotentAndCredentialsWork(t *testing.T) {
	r, pool := newRepo(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := r.SeedDemo(ctx); err != nil {
			t.Fatalf("SeedDemo #%d: %v", i+1, err)
		}
	}
	if got := rowCount(t, pool, "TRUE"); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	demo, err := r.FindByUsername(ctx, "demo")
	if err != nil || demo.Role != "user" || !authx.VerifyPassword(demo.PasswordHash, "demo1234") {
		t.Fatalf("demo = %+v %v", demo, err)
	}
	admin, err := r.FindByUsername(ctx, "admin")
	if err != nil || admin.Role != "admin" || !authx.VerifyPassword(admin.PasswordHash, "admin1234") {
		t.Fatalf("admin = %+v %v", admin, err)
	}
}

func TestSeedDemoDoesNotOverwriteChangedUser(t *testing.T) {
	r, pool := newRepo(t)
	ctx := context.Background()
	if err := r.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE usr.users SET password_hash = 'changed' WHERE username = 'demo'"); err != nil {
		t.Fatal(err)
	}
	if err := r.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if u, _ := r.FindByUsername(ctx, "demo"); u.PasswordHash != "changed" {
		t.Fatal("重复执行种子不得覆盖已被修改的账号")
	}
}

func TestBatchGenerateTenThousandUsersThroughRealRepo(t *testing.T) {
	r, pool := newRepo(t)
	svc := newService(t, r)
	ctx := context.Background()

	start := time.Now()
	res, err := svc.GenerateBatch(ctx, service.BatchInput{Prefix: "load", Count: 10000})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if res.Created != 10000 || res.Existing != 0 {
		t.Fatalf("res = %+v", res)
	}
	t.Logf("10000 users created in %v", elapsed)
	if elapsed > 20*time.Second {
		t.Fatalf("1 万用户耗时 %v，太慢（应共用一个 bcrypt 哈希并分块插入）", elapsed)
	}
	if got := rowCount(t, pool, "username LIKE 'load%' AND role = 'user'"); got != 10000 {
		t.Fatalf("rows = %d, want 10000", got)
	}

	again, err := svc.GenerateBatch(ctx, service.BatchInput{Prefix: "load", Count: 10000})
	if err != nil || again.Created != 0 || again.Existing != 10000 {
		t.Fatalf("重跑应是空操作: %+v %v", again, err)
	}
	extend, err := svc.GenerateBatch(ctx, service.BatchInput{Prefix: "load", Start: 9990, Count: 20})
	if err != nil || extend.Created != 10 || extend.Existing != 10 {
		t.Fatalf("重叠扩展应只新建 10 个: %+v %v", extend, err)
	}

	u, err := r.FindByUsername(ctx, "load42")
	if err != nil || !authx.VerifyPassword(u.PasswordHash, service.DefaultBatchPassword) {
		t.Fatalf("批量用户应能用默认口令登录: %+v %v", u, err)
	}
}
