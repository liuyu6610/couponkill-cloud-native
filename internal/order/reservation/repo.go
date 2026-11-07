package reservation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// columns 是 Reservation 的全部列，顺序与 scan 一致。
const columns = `id, user_id, coupon_id, status, reserve_at, trigger_at, fired_at, request_id, order_id,
	fail_code, fail_reason, retry_count, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

// scan 按 columns 的顺序读取一行。
func scan(s scanner) (Reservation, error) {
	var r Reservation
	var status string
	err := s.Scan(&r.ID, &r.UserID, &r.CouponID, &status, &r.ReserveAt, &r.TriggerAt, &r.FiredAt, &r.RequestID,
		&r.OrderID, &r.FailCode, &r.FailReason, &r.RetryCount, &r.CreatedAt, &r.UpdatedAt)
	r.Status = Status(status)
	return r, err
}

// insertPending 写入一条 PENDING 预约。同一用户同一券已有进行中的预约时返回唯一冲突错误
// （由调用方用 dbx.IsUniqueViolation 判断）。
func insertPending(ctx context.Context, q DBTX, r Reservation) error {
	_, err := q.Exec(ctx,
		`INSERT INTO ord.reservations (id, user_id, coupon_id, status, reserve_at, trigger_at, retry_count, created_at, updated_at)
		 VALUES ($1, $2, $3, 'PENDING', $4, $5, 0, $4, $4)`,
		r.ID, r.UserID, r.CouponID, r.ReserveAt, r.TriggerAt)
	if err != nil {
		return fmt.Errorf("写入预约: %w", err)
	}
	return nil
}

// getByID 读取一条预约。userID 非 0 时附带属主谓词（别人的预约视同不存在）。不存在返回 ErrNotFound。
func getByID(ctx context.Context, q DBTX, id, userID int64) (Reservation, error) {
	r, err := scan(q.QueryRow(ctx,
		`SELECT `+columns+` FROM ord.reservations WHERE id = $1 AND ($2::bigint = 0 OR user_id = $2)`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, fmt.Errorf("读取预约: %w", err)
	}
	return r, nil
}

// listByUser 返回 userID 的预约，最新在前（命中 idx_reservations_user_created）。
func listByUser(ctx context.Context, q DBTX, userID int64, page, size int) ([]Reservation, int64, error) {
	var total int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM ord.reservations WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计预约: %w", err)
	}
	rows, err := q.Query(ctx,
		`SELECT `+columns+` FROM ord.reservations WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("查询预约: %w", err)
	}
	defer rows.Close()
	items := make([]Reservation, 0, size)
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("读取预约: %w", err)
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历预约: %w", err)
	}
	return items, total, nil
}

// change 描述一次状态迁移及随之写入的列；nil/零值的可选列保持原值。
type change struct {
	id  int64
	to  Status
	now time.Time
	// userID 非 0 时要求属主匹配（用户取消用）。
	userID int64
	// fence 非 nil 时要求 retry_count 等于它：认领方凭它确认“行仍是我认领的那一轮”，
	// 被崩溃回收并重新认领后 retry_count 已变，迟到的写入因此落空。
	fence *int

	requestID  *string
	orderID    *int64
	failCode   *int
	failReason *string
	triggerAt  *time.Time
	firedAt    *time.Time
	// bumpRetry 为 true 时 retry_count 加一。
	bumpRetry bool
}

// move 是所有状态迁移的唯一入口：UPDATE 的 WHERE 带上由 transitions 推导出的合法来源状态，
// 因此非法迁移在数据库里就是“0 行受影响”。返回更新后的行与是否真正迁移。
func move(ctx context.Context, q DBTX, ch change) (Reservation, bool, error) {
	if !ch.to.Valid() {
		return Reservation{}, false, fmt.Errorf("非法的目标状态 %q", ch.to)
	}
	bump := 0
	if ch.bumpRetry {
		bump = 1
	}
	var owner *int64
	if ch.userID != 0 {
		owner = &ch.userID
	}
	r, err := scan(q.QueryRow(ctx,
		`UPDATE ord.reservations SET
			status = $2,
			updated_at = $3,
			request_id = COALESCE($4, request_id),
			order_id = COALESCE($5, order_id),
			fail_code = COALESCE($6, fail_code),
			fail_reason = COALESCE($7, fail_reason),
			trigger_at = COALESCE($8, trigger_at),
			fired_at = COALESCE($9, fired_at),
			retry_count = retry_count + $10
		 WHERE id = $1
		   AND status = ANY($11::text[])
		   AND ($12::int IS NULL OR retry_count = $12)
		   AND ($13::bigint IS NULL OR user_id = $13)
		 RETURNING `+columns,
		ch.id, string(ch.to), ch.now, ch.requestID, ch.orderID, ch.failCode, ch.failReason,
		ch.triggerAt, ch.firedAt, bump, sourcesOf(ch.to), ch.fence, owner))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, false, nil
	}
	if err != nil {
		return Reservation{}, false, fmt.Errorf("迁移预约 %d 到 %s: %w", ch.id, ch.to, err)
	}
	return r, true, nil
}

// claimed 是被调度器认领（PENDING→FIRING）的一条预约的最小信息。
type claimed struct {
	id, userID, couponID int64
	// retryCount 是认领时的 retry_count，后续写入以它作 fence。
	retryCount int
}

func scanClaimed(s scanner) (claimed, error) {
	var c claimed
	err := s.Scan(&c.id, &c.userID, &c.couponID, &c.retryCount)
	return c, err
}

// claimDue 在一条语句里认领至多 limit 条到点的 PENDING 预约（置 FIRING、写 fired_at）。
// 子查询带 FOR UPDATE SKIP LOCKED：多个副本并发认领时各拿各的行，互不阻塞、不会拿到同一行。
func claimDue(ctx context.Context, q DBTX, now time.Time, limit int) ([]claimed, error) {
	rows, err := q.Query(ctx,
		`WITH due AS (
			SELECT id FROM ord.reservations
			WHERE status = 'PENDING' AND trigger_at <= $1
			ORDER BY trigger_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE ord.reservations r
		SET status = 'FIRING', fired_at = $1, updated_at = $1
		FROM due WHERE r.id = due.id
		RETURNING r.id, r.user_id, r.coupon_id, r.retry_count`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("认领到点预约: %w", err)
	}
	defer rows.Close()
	var out []claimed
	for rows.Next() {
		c, err := scanClaimed(rows)
		if err != nil {
			return nil, fmt.Errorf("读取认领结果: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历认领结果: %w", err)
	}
	return out, nil
}

// touch 在调用秒杀入口之前刷新 fired_at，同时确认这一行仍是自己认领的那一轮（status=FIRING 且
// retry_count 未变）。返回 false 表示已被崩溃回收/他人接手，调用方必须放弃，不得再调用入口。
func touch(ctx context.Context, q DBTX, c claimed, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx,
		`UPDATE ord.reservations SET fired_at = $2, updated_at = $2
		 WHERE id = $1 AND status = 'FIRING' AND retry_count = $3`, c.id, now, c.retryCount)
	if err != nil {
		return false, fmt.Errorf("刷新认领时间: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// lockStale 锁定（SKIP LOCKED）至多 limit 条认领后超过 cutoff 仍停在 FIRING 的预约，必须在事务内调用。
func lockStale(ctx context.Context, q DBTX, cutoff time.Time, limit int) ([]claimed, error) {
	rows, err := q.Query(ctx,
		`SELECT id, user_id, coupon_id, retry_count FROM ord.reservations
		 WHERE status = 'FIRING' AND fired_at < $1
		 ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("查找卡住的预约: %w", err)
	}
	defer rows.Close()
	var out []claimed
	for rows.Next() {
		c, err := scanClaimed(rows)
		if err != nil {
			return nil, fmt.Errorf("读取卡住的预约: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历卡住的预约: %w", err)
	}
	return out, nil
}

// lockForResult 按结果事件锁定对应的预约，必须在事务内调用：
// 优先按 request_id 精确匹配；否则兜底匹配“同一用户同一券、仍在 FIRING 且尚未记录 request_id”的预约
// （结果事件先于 QUEUED 回写到达）。两个分支放在同一条语句里：调度器同时在改这一行时，
// FOR UPDATE 等锁后会按新版本重新求值谓词，所以不会漏掉任何一种交错。
func lockForResult(ctx context.Context, q DBTX, ev ResultEvent) (Reservation, bool, error) {
	r, err := scan(q.QueryRow(ctx,
		`SELECT `+columns+` FROM ord.reservations
		 WHERE request_id = $1
		    OR (status = 'FIRING' AND request_id IS NULL AND user_id = $2 AND coupon_id = $3)
		 ORDER BY (request_id IS NOT NULL) DESC
		 LIMIT 1 FOR UPDATE`, ev.RequestID, ev.UserID, ev.CouponID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, false, nil
	}
	if err != nil {
		return Reservation{}, false, fmt.Errorf("按结果查找预约: %w", err)
	}
	return r, true, nil
}

// queuedRef 是待回查结果的 QUEUED 预约。
type queuedRef struct {
	id, userID, couponID int64
	requestID            string
}

// lockQueuedStale 锁定（SKIP LOCKED）至多 limit 条 updated_at 早于 cutoff 的 QUEUED 预约，
// 并在同一条语句里把它们的 updated_at 推到 now（下一轮回查要等 QueuedSyncAfter 之后）。
func lockQueuedStale(ctx context.Context, q DBTX, cutoff, now time.Time, limit int) ([]queuedRef, error) {
	rows, err := q.Query(ctx,
		`WITH picked AS (
			SELECT id FROM ord.reservations
			WHERE status = 'QUEUED' AND request_id IS NOT NULL AND updated_at < $1
			ORDER BY updated_at LIMIT $2 FOR UPDATE SKIP LOCKED
		)
		UPDATE ord.reservations r SET updated_at = $3
		FROM picked WHERE r.id = picked.id
		RETURNING r.id, r.user_id, r.coupon_id, r.request_id`, cutoff, limit, now)
	if err != nil {
		return nil, fmt.Errorf("查找待回查的预约: %w", err)
	}
	defer rows.Close()
	var out []queuedRef
	for rows.Next() {
		var ref queuedRef
		if err := rows.Scan(&ref.id, &ref.userID, &ref.couponID, &ref.requestID); err != nil {
			return nil, fmt.Errorf("读取待回查的预约: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待回查的预约: %w", err)
	}
	return out, nil
}
