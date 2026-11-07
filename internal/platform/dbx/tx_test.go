package dbx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// fakeTx 只实现 WithTx 用到的 Commit/Rollback，其余方法嵌入 nil 接口（被调用即 panic，说明越界使用）。
type fakeTx struct {
	pgx.Tx
	db *fakeDB
}

func (f *fakeTx) Commit(context.Context) error {
	f.db.commits++
	if f.db.commitErr != nil && f.db.commits <= f.db.commitFailures {
		return f.db.commitErr
	}
	return nil
}

func (f *fakeTx) Rollback(context.Context) error {
	f.db.rollbacks++
	return f.db.rollbackErr
}

type fakeDB struct {
	begins         int
	commits        int
	rollbacks      int
	beginErr       error
	commitErr      error
	commitFailures int
	rollbackErr    error
	lastOpts       pgx.TxOptions
}

func (d *fakeDB) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	d.begins++
	d.lastOpts = o
	if d.beginErr != nil {
		return nil, d.beginErr
	}
	return &fakeTx{db: d}, nil
}

func TestWithTxCommitsOnSuccess(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	err := WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error { calls++; return nil })
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if calls != 1 || db.commits != 1 || db.rollbacks != 0 {
		t.Fatalf("calls=%d commits=%d rollbacks=%d", calls, db.commits, db.rollbacks)
	}
}

func TestWithTxPassesIsolationLevel(t *testing.T) {
	db := &fakeDB{}
	opts := TxOptions{Isolation: pgx.Serializable, AccessMode: pgx.ReadOnly}
	if err := WithTx(context.Background(), db, opts, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if db.lastOpts.IsoLevel != pgx.Serializable || db.lastOpts.AccessMode != pgx.ReadOnly {
		t.Fatalf("事务选项未透传: %+v", db.lastOpts)
	}
}

func TestWithTxRollsBackAndReturnsCallbackError(t *testing.T) {
	db := &fakeDB{}
	boom := errors.New("boom")
	err := WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("应返回回调错误, got %v", err)
	}
	if db.commits != 0 || db.rollbacks != 1 {
		t.Fatalf("commits=%d rollbacks=%d", db.commits, db.rollbacks)
	}
}

func TestWithTxRollsBackAndRepanics(t *testing.T) {
	db := &fakeDB{}
	defer func() {
		r := recover()
		if r != "kaboom" {
			t.Fatalf("应原样重新抛出 panic, got %v", r)
		}
		if db.commits != 0 || db.rollbacks != 1 {
			t.Fatalf("panic 时必须回滚: commits=%d rollbacks=%d", db.commits, db.rollbacks)
		}
	}()
	_ = WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error { panic("kaboom") })
	t.Fatal("不应走到这里")
}

func TestWithTxBeginErrorIsWrapped(t *testing.T) {
	beginErr := errors.New("pool exhausted")
	db := &fakeDB{beginErr: beginErr}
	err := WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error { return nil })
	if !errors.Is(err, beginErr) {
		t.Fatalf("应保留 Begin 错误链, got %v", err)
	}
}

func TestWithTxRetriesSerializationFailure(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	opts := TxOptions{MaxRetries: 3, RetryBackoff: time.Millisecond}
	err := WithTx(context.Background(), db, opts, func(pgx.Tx) error {
		calls++
		if calls < 3 {
			return pgErr("40001", "")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("第 3 次应成功: %v", err)
	}
	if calls != 3 || db.begins != 3 || db.rollbacks != 2 || db.commits != 1 {
		t.Fatalf("calls=%d begins=%d rollbacks=%d commits=%d", calls, db.begins, db.rollbacks, db.commits)
	}
}

func TestWithTxRetriesDeadlock(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	opts := TxOptions{MaxRetries: 1, RetryBackoff: time.Millisecond}
	err := WithTx(context.Background(), db, opts, func(pgx.Tx) error {
		calls++
		if calls == 1 {
			return pgErr("40P01", "")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestWithTxRetryOffByDefault(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	err := WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error {
		calls++
		return pgErr("40001", "")
	})
	if !IsSerializationFailure(err) || calls != 1 {
		t.Fatalf("默认不重试: err=%v calls=%d", err, calls)
	}
}

func TestWithTxRetryExhaustedReturnsLastError(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	opts := TxOptions{MaxRetries: 2, RetryBackoff: time.Millisecond}
	err := WithTx(context.Background(), db, opts, func(pgx.Tx) error {
		calls++
		return pgErr("40001", "")
	})
	if !IsSerializationFailure(err) {
		t.Fatalf("重试耗尽应返回序列化失败, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("1 次初始 + 2 次重试 = 3, got %d", calls)
	}
}

func TestWithTxDoesNotRetryNonRetryable(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	opts := TxOptions{MaxRetries: 5, RetryBackoff: time.Millisecond}
	err := WithTx(context.Background(), db, opts, func(pgx.Tx) error {
		calls++
		return pgErr("23505", "uk")
	})
	if !IsUniqueViolation(err, "uk") || calls != 1 {
		t.Fatalf("唯一冲突不应重试: err=%v calls=%d", err, calls)
	}
}

func TestWithTxRetriesCommitSerializationFailure(t *testing.T) {
	db := &fakeDB{commitErr: pgErr("40001", ""), commitFailures: 1}
	calls := 0
	opts := TxOptions{MaxRetries: 1, RetryBackoff: time.Millisecond}
	err := WithTx(context.Background(), db, opts, func(pgx.Tx) error { calls++; return nil })
	if err != nil || calls != 2 {
		t.Fatalf("提交阶段的序列化失败也应重试: err=%v calls=%d", err, calls)
	}
}

func TestWithTxStopsRetryWhenContextCancelled(t *testing.T) {
	db := &fakeDB{}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	opts := TxOptions{MaxRetries: 10, RetryBackoff: time.Hour}
	err := WithTx(ctx, db, opts, func(pgx.Tx) error {
		calls++
		cancel()
		return pgErr("40001", "")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("退避期间 ctx 取消应返回 context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("取消后不应再执行回调, calls=%d", calls)
	}
}

func TestWithTxRollbackFailureDoesNotMaskCallbackError(t *testing.T) {
	db := &fakeDB{rollbackErr: errors.New("conn reset")}
	boom := errors.New("boom")
	err := WithTx(context.Background(), db, TxOptions{}, func(pgx.Tx) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("回滚失败不应覆盖业务错误, got %v", err)
	}
}
