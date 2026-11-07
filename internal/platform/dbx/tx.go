package dbx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultRetryBackoff = 10 * time.Millisecond
	maxRetryBackoff     = time.Second
	rollbackTimeout     = 5 * time.Second
)

// TxBeginner 是 WithTx 需要的最小能力；*pgxpool.Pool、*pgxpool.Conn、*pgx.Conn 都满足它。
type TxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// TxOptions 控制 WithTx 的事务属性与重试策略。零值 = 默认隔离级别、读写、不重试。
type TxOptions struct {
	// Isolation 事务隔离级别，空串表示使用库默认（read committed）。
	Isolation pgx.TxIsoLevel
	// AccessMode 读写模式，空串表示读写。
	AccessMode pgx.TxAccessMode
	// MaxRetries 遇到序列化失败（40001）或死锁（40P01）时最多重试的次数；0 表示不重试（默认）。
	// 回调可能被执行多次，因此重试开启时回调必须只做事务内的数据库操作，不得有外部副作用。
	MaxRetries int
	// RetryBackoff 首次重试前的退避基数（之后指数增长并带抖动，上限 1s）；0 表示 10ms。
	RetryBackoff time.Duration
}

// WithTx 在一个事务里执行 fn：fn 返回 nil 则提交，返回错误则回滚并原样返回该错误；
// fn panic 时先回滚再把 panic 重新抛出，保证连接不会带着未结束的事务回到连接池。
//
// opts.MaxRetries > 0 时，fn 返回的或提交阶段产生的“序列化失败/死锁”会整体重试，
// 退避期间 ctx 取消则立即返回 ctx 的错误。其他错误不重试。
func WithTx(ctx context.Context, db TxBeginner, opts TxOptions, fn func(tx pgx.Tx) error) error {
	for attempt := 0; ; attempt++ {
		err := runTx(ctx, db, opts, fn)
		if err == nil || !IsRetryable(err) || attempt >= opts.MaxRetries {
			return err
		}
		slog.WarnContext(ctx, "事务因序列化失败/死锁重试", "attempt", attempt+1, "max_retries", opts.MaxRetries, "error", err)
		if waitErr := sleepCtx(ctx, backoffFor(opts.RetryBackoff, attempt)); waitErr != nil {
			return fmt.Errorf("事务重试被取消: %w", waitErr)
		}
	}
}

// runTx 执行一次“开启 → fn → 提交/回滚”。
func runTx(ctx context.Context, db TxBeginner, opts TxOptions, fn func(tx pgx.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: opts.Isolation, AccessMode: opts.AccessMode})
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		rollback(ctx, tx)
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务: %w", err)
	}
	committed = true
	return nil
}

// rollback 用不随调用方 ctx 取消的短超时上下文回滚，避免 ctx 已取消时连接带着事务归还。
func rollback(ctx context.Context, tx pgx.Tx) {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	if err := tx.Rollback(rbCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.WarnContext(ctx, "事务回滚失败", "error", err)
	}
}

// backoffFor 返回第 attempt 次重试前的等待：base*2^attempt，上限 1s，再叠加 0~50% 抖动。
func backoffFor(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = defaultRetryBackoff
	}
	d := base
	for i := 0; i < attempt && d < maxRetryBackoff; i++ {
		d *= 2
	}
	if d > maxRetryBackoff {
		d = maxRetryBackoff
	}
	return d + time.Duration(rand.Int64N(int64(d)/2+1))
}

// sleepCtx 睡眠 d，ctx 取消时提前返回 ctx.Err()。
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
