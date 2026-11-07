// Package repo 是 coupon 服务的 PostgreSQL 实现（schema cpn），满足 service.Repo。
//
// # 库存不变式
//
// 库存拆在 N 个桶里（cpn.coupon_stock），每次扣减/归还都与一行 cpn.stock_ledger 在同一个事务里提交，
// 以 request_id 为主键，因此：
//
//   - 同一 requestId 至多扣一次：流水行先于桶更新「占位」（INSERT ... ON CONFLICT DO NOTHING），
//     并发的同 requestId 调用会在唯一索引上排队，后到者看到已提交的流水后直接返回第一次的桶；
//   - 崩溃安全：占位、桶更新、回填桶号在一个事务里，进程在任何一步崩溃都整体回滚，不会出现
//     「桶扣了但没有流水」或「有流水但桶没扣」；
//   - 售罄 ≠ 冲突：从 hash(userId)%N 桶起循环扫描，只有每个桶都没有库存才返回 OutOfStock，
//     桶被别的事务锁住时是等待，不是失败，也不存在版本号/乐观锁；
//   - 已归还的 requestId 再扣返回 OutOfStock（墓碑），归还重复调用是空操作。
//
// 桶编号约定为 0..N-1 连续：创建时一次性写入，同步只改数量不改桶数，因此扣减按「现有桶数」取模，
// 不受 CK_COUPON_STOCK_BUCKETS 事后改动的影响。
package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/contracts/internalapi"
	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
)

// 流水状态。
const (
	stateDeducted = "deducted"
	stateReleased = "released"
)

// 约束名，见 migrations/0001_init.sql。
const constraintCouponsPK = "pk_coupons"

// Repo 是基于 pgx 连接池的 service.Repo 实现，并发安全。
type Repo struct {
	pool *pgxpool.Pool
	// afterBucketUpdate 仅供测试注入「桶已更新、事务未提交」时的崩溃；生产为 nil。
	afterBucketUpdate func() error
}

var _ service.Repo = (*Repo)(nil)

// New 创建 Repo。调用方负责先对 cpn schema 执行迁移，并在退出时关闭连接池。
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// couponColumns 是读模型的列，剩余量由库存桶汇总。
const couponColumns = `c.id, c.name, COALESCE(c.description, ''), c.type, c.status,
	c.face_value::text, c.min_spend::text, c.valid_days, c.per_user_limit, c.total_stock,
	COALESCE((SELECT sum(s.remaining) FROM cpn.coupon_stock s WHERE s.coupon_id = c.id), 0)::bigint,
	c.seckill_start_at, c.seckill_end_at, c.created_at, c.updated_at`

// scanCoupon 把一行 couponColumns 扫描成 service.Coupon，时间统一为 UTC。
func scanCoupon(row pgx.Row) (service.Coupon, error) {
	var (
		c          service.Coupon
		start, end *time.Time
	)
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &c.Type, &c.Status, &c.FaceValue, &c.MinSpend,
		&c.ValidDays, &c.PerUserLimit, &c.TotalStock, &c.Remaining, &start, &end, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return service.Coupon{}, err
	}
	c.SeckillStartAt, c.SeckillEndAt = utcPtr(start), utcPtr(end)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return c, nil
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (r *Repo) queryCoupons(ctx context.Context, sql string, args ...any) ([]service.Coupon, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("查询券: %w", err)
	}
	defer rows.Close()
	var out []service.Coupon
	for rows.Next() {
		c, err := scanCoupon(rows)
		if err != nil {
			return nil, fmt.Errorf("扫描券: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历券: %w", err)
	}
	return out, nil
}

// Available 返回启用且秒杀时间窗尚未结束的券，按创建时间倒序。
func (r *Repo) Available(ctx context.Context, now time.Time, limit int) ([]service.Coupon, error) {
	return r.queryCoupons(ctx, `SELECT `+couponColumns+` FROM cpn.coupons c
		WHERE c.status = 1 AND (c.seckill_end_at IS NULL OR c.seckill_end_at > $1)
		ORDER BY c.created_at DESC, c.id DESC LIMIT $2`, now, limit)
}

// List 分页返回券与总数。
func (r *Repo) List(ctx context.Context, offset, limit int, enabledOnly bool) ([]service.Coupon, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM cpn.coupons WHERE ($1::bool = false OR status = 1)`, enabledOnly).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计券数量: %w", err)
	}
	items, err := r.queryCoupons(ctx, `SELECT `+couponColumns+` FROM cpn.coupons c
		WHERE ($1::bool = false OR c.status = 1)
		ORDER BY c.created_at DESC, c.id DESC LIMIT $2 OFFSET $3`, enabledOnly, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get 按 ID 读取一张券；不存在返回 apperr.CouponNotFound。
func (r *Repo) Get(ctx context.Context, id int64) (service.Coupon, error) {
	c, err := scanCoupon(r.pool.QueryRow(ctx, `SELECT `+couponColumns+` FROM cpn.coupons c WHERE c.id = $1`, id))
	if dbx.IsNoRows(err) {
		return service.Coupon{}, apperr.CouponNotFound
	}
	if err != nil {
		return service.Coupon{}, fmt.Errorf("读取券 %d: %w", id, err)
	}
	return c, nil
}

// Create 在一个事务里写入券与各库存桶；ID 已存在返回 409。
func (r *Repo) Create(ctx context.Context, nc service.NewCoupon, buckets []int64) (service.Coupon, error) {
	if len(buckets) == 0 {
		return service.Coupon{}, errors.New("创建券至少需要一个库存桶")
	}
	err := dbx.WithTx(ctx, r.pool, dbx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO cpn.coupons
			(id, name, description, type, face_value, min_spend, valid_days, per_user_limit, total_stock, status, seckill_start_at, seckill_end_at)
			VALUES ($1, $2, NULLIF($3::text, ''), $4, $5::text::numeric, $6::text::numeric, $7, $8, $9, 1, $10, $11)`,
			nc.ID, nc.Name, nc.Description, nc.Type, nc.FaceValue, nc.MinSpend, nc.ValidDays, nc.PerUserLimit,
			nc.TotalStock, nc.SeckillStartAt, nc.SeckillEndAt); err != nil {
			return err
		}
		return insertBuckets(ctx, tx, nc.ID, buckets)
	})
	if dbx.IsUniqueViolation(err, constraintCouponsPK) {
		return service.Coupon{}, apperr.New(apperr.CodeInvalidRequest, http.StatusConflict, "优惠券ID已存在")
	}
	if err != nil {
		return service.Coupon{}, fmt.Errorf("写入券 %d: %w", nc.ID, err)
	}
	return r.Get(ctx, nc.ID)
}

// insertBuckets 把 amounts[i] 写成第 i 桶（已存在则覆盖），同一语句内按桶号顺序加锁。
func insertBuckets(ctx context.Context, tx pgx.Tx, couponID int64, amounts []int64) error {
	ids := make([]int16, len(amounts))
	vals := make([]int32, len(amounts))
	for i, a := range amounts {
		if a < 0 || a > math.MaxInt32 {
			return fmt.Errorf("桶 %d 的库存 %d 超出范围", i, a)
		}
		ids[i], vals[i] = int16(i), int32(a)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cpn.coupon_stock (coupon_id, bucket, remaining)
		SELECT $1, t.b, t.r FROM unnest($2::smallint[], $3::int[]) AS t(b, r) ORDER BY t.b
		ON CONFLICT (coupon_id, bucket) DO UPDATE SET remaining = EXCLUDED.remaining`, couponID, ids, vals); err != nil {
		return fmt.Errorf("写入库存桶: %w", err)
	}
	return nil
}

// UpdateWindow 设置秒杀时间窗。
func (r *Repo) UpdateWindow(ctx context.Context, id int64, start, end time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE cpn.coupons SET seckill_start_at = $2, seckill_end_at = $3, updated_at = now() WHERE id = $1`, id, start, end)
	return rowOrNotFound(tag.RowsAffected(), err, "更新时间窗")
}

// UpdateStatus 设置券状态。
func (r *Repo) UpdateStatus(ctx context.Context, id int64, status int) error {
	tag, err := r.pool.Exec(ctx, `UPDATE cpn.coupons SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return rowOrNotFound(tag.RowsAffected(), err, "更新状态")
}

func rowOrNotFound(rows int64, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if rows == 0 {
		return apperr.CouponNotFound
	}
	return nil
}

// Delete 删除券（库存桶随外键级联删除，库存流水保留），返回删除的券数。
func (r *Repo) Delete(ctx context.Context, id int64) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cpn.coupons WHERE id = $1`, id)
	if err != nil {
		return 0, fmt.Errorf("删除券 %d: %w", id, err)
	}
	return int(tag.RowsAffected()), nil
}

// StockState 读取类型、状态与剩余库存。
func (r *Repo) StockState(ctx context.Context, id int64) (service.StockState, error) {
	var st service.StockState
	err := r.pool.QueryRow(ctx, `SELECT c.type, c.status,
		COALESCE((SELECT sum(s.remaining) FROM cpn.coupon_stock s WHERE s.coupon_id = c.id), 0)::bigint
		FROM cpn.coupons c WHERE c.id = $1`, id).Scan(&st.Type, &st.Status, &st.Remaining)
	if dbx.IsNoRows(err) {
		return service.StockState{}, apperr.CouponNotFound
	}
	if err != nil {
		return service.StockState{}, fmt.Errorf("读取券 %d 的库存状态: %w", id, err)
	}
	return st, nil
}

// EnabledSeckillIDs 返回所有启用中的秒杀券 ID（升序）。
func (r *Repo) EnabledSeckillIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM cpn.coupons WHERE type = 2 AND status = 1 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询启用中的秒杀券: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("扫描券 ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历券 ID: %w", err)
	}
	return ids, nil
}

// stockTxOptions 是库存事务（扣减、归还、同步）的选项：遇到死锁或序列化失败时整体重试。
//
// 死锁在这里是真实存在的：Deduct 从 hash(userId)%N 桶起环形扫描，而 SetAvailable 按桶号顺序锁桶，
// PostgreSQL 复核并发更新的行时即使最终不匹配也会保留行锁，两边加锁顺序不一致就可能成环。
// 这三个事务都在单个事务里完成且对同一 requestId / 目标值幂等，被选为牺牲品而回滚后重来是安全的，
// 回调开头都会重置结果变量。
var stockTxOptions = dbx.TxOptions{MaxRetries: 5}

// SetAvailable 在一个事务里按 mode 调整可用库存，返回实际生效值。
//
// 先锁券行，再按桶号顺序锁住全部库存桶并读出当前剩余量（并发的扣减 UPDATE 会在桶行锁上排队，
// 所以「当前值」与「改写」之间没有空隙）：
//   - SyncModeSet：按现有桶数（没有则用 defaultBuckets）重新均衡分配 available，并把发行总量抬到不小于它；
//   - SyncModeMin：目标不低于当前剩余量则什么都不改，返回当前值；低于则降到目标并重新均衡。
//
// 券还没有任何库存桶时没有当前值可保护，两种模式都按首次设置处理。
func (r *Repo) SetAvailable(ctx context.Context, id int64, available int64, defaultBuckets int, mode string) (int64, error) {
	var applied int64
	err := dbx.WithTx(ctx, r.pool, stockTxOptions, func(tx pgx.Tx) error {
		applied = available
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM cpn.coupons WHERE id = $1 FOR UPDATE`, id).Scan(&one); err != nil {
			if dbx.IsNoRows(err) {
				return apperr.CouponNotFound
			}
			return fmt.Errorf("锁定券 %d: %w", id, err)
		}
		n, current, err := lockBuckets(ctx, tx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			n = defaultBuckets
		} else if mode == internalapi.SyncModeMin && available >= current {
			applied = current
			return nil
		}
		if err := insertBuckets(ctx, tx, id, service.Distribute(applied, n)); err != nil {
			return err
		}
		if mode == internalapi.SyncModeSet {
			if _, err := tx.Exec(ctx, `UPDATE cpn.coupons SET total_stock = GREATEST(total_stock, $2), updated_at = now() WHERE id = $1`, id, applied); err != nil {
				return fmt.Errorf("抬高发行总量: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return applied, nil
}

// lockBuckets 按桶号顺序锁住券的全部库存桶，返回桶数与当前剩余量之和。
func lockBuckets(ctx context.Context, tx pgx.Tx, id int64) (n int, remaining int64, err error) {
	rows, err := tx.Query(ctx, `SELECT remaining FROM cpn.coupon_stock WHERE coupon_id = $1 ORDER BY bucket FOR UPDATE`, id)
	if err != nil {
		return 0, 0, fmt.Errorf("锁定库存桶: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rem int64
		if err := rows.Scan(&rem); err != nil {
			return 0, 0, fmt.Errorf("读取桶剩余量: %w", err)
		}
		n++
		remaining += rem
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("遍历库存桶: %w", err)
	}
	return n, remaining, nil
}

// Deduct 见包文档：占位流水 → 校验券 → 从 hash(userId)%N 桶起循环扣减 → 回填桶号，全在一个事务里。
func (r *Repo) Deduct(ctx context.Context, req internalapi.DeductRequest) (service.DeductResult, error) {
	var res service.DeductResult
	err := dbx.WithTx(ctx, r.pool, stockTxOptions, func(tx pgx.Tx) error {
		res = service.DeductResult{}
		claim, err := tx.Exec(ctx, `INSERT INTO cpn.stock_ledger (request_id, coupon_id, bucket, user_id, state)
			VALUES ($1, $2, 0, $3, 'deducted') ON CONFLICT (request_id) DO NOTHING`, req.RequestID, req.CouponID, req.UserID)
		if err != nil {
			return fmt.Errorf("占位库存流水: %w", err)
		}
		if claim.RowsAffected() == 0 {
			return replayDeduct(ctx, tx, req, &res)
		}

		var status, buckets int
		err = tx.QueryRow(ctx, `SELECT c.status, (SELECT count(*) FROM cpn.coupon_stock s WHERE s.coupon_id = c.id)::int
			FROM cpn.coupons c WHERE c.id = $1`, req.CouponID).Scan(&status, &buckets)
		if dbx.IsNoRows(err) {
			return apperr.CouponNotFound
		}
		if err != nil {
			return fmt.Errorf("读取券 %d: %w", req.CouponID, err)
		}
		if status != service.StatusEnabled {
			return apperr.CouponDisabled
		}
		if buckets == 0 {
			return apperr.OutOfStock
		}

		start := startBucket(req.UserID, buckets)
		for i := range buckets {
			b := (start + i) % buckets
			tag, err := tx.Exec(ctx, `UPDATE cpn.coupon_stock SET remaining = remaining - 1
				WHERE coupon_id = $1 AND bucket = $2 AND remaining > 0`, req.CouponID, int16(b))
			if err != nil {
				return fmt.Errorf("扣减桶 %d: %w", b, err)
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			if r.afterBucketUpdate != nil {
				if err := r.afterBucketUpdate(); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE cpn.stock_ledger SET bucket = $2, updated_at = now() WHERE request_id = $1`, req.RequestID, int16(b)); err != nil {
				return fmt.Errorf("回填流水桶号: %w", err)
			}
			res.Bucket = b
			return nil
		}
		return apperr.OutOfStock
	})
	if err != nil {
		return service.DeductResult{}, err
	}
	return res, nil
}

// replayDeduct 处理 requestId 已有流水的情况：已扣返回原桶，已归还视为售罄，券不一致视为非法请求。
func replayDeduct(ctx context.Context, tx pgx.Tx, req internalapi.DeductRequest, res *service.DeductResult) error {
	var (
		couponID int64
		bucket   int
		state    string
	)
	if err := tx.QueryRow(ctx, `SELECT coupon_id, bucket, state FROM cpn.stock_ledger WHERE request_id = $1`, req.RequestID).
		Scan(&couponID, &bucket, &state); err != nil {
		return fmt.Errorf("读取库存流水: %w", err)
	}
	if couponID != req.CouponID {
		return apperr.InvalidRequest.WithMessage("requestId 已用于其他优惠券")
	}
	if state == stateReleased {
		return apperr.OutOfStock
	}
	*res = service.DeductResult{Bucket: bucket, Already: true}
	return nil
}

// Release 见包文档：占位墓碑；若已有流水则按其桶回补并置为已归还，全在一个事务里。
func (r *Repo) Release(ctx context.Context, req internalapi.ReleaseRequest) (service.ReleaseResult, error) {
	var res service.ReleaseResult
	err := dbx.WithTx(ctx, r.pool, stockTxOptions, func(tx pgx.Tx) error {
		res = service.ReleaseResult{}
		tomb, err := tx.Exec(ctx, `INSERT INTO cpn.stock_ledger (request_id, coupon_id, bucket, user_id, state)
			VALUES ($1, $2, 0, 0, 'released') ON CONFLICT (request_id) DO NOTHING`, req.RequestID, req.CouponID)
		if err != nil {
			return fmt.Errorf("写入归还墓碑: %w", err)
		}
		if tomb.RowsAffected() == 1 {
			return nil // 从未扣减：墓碑已写入，迟到的 deduct 会被拒绝
		}

		var (
			couponID int64
			bucket   int
			state    string
		)
		if err := tx.QueryRow(ctx, `SELECT coupon_id, bucket, state FROM cpn.stock_ledger WHERE request_id = $1 FOR UPDATE`, req.RequestID).
			Scan(&couponID, &bucket, &state); err != nil {
			return fmt.Errorf("读取库存流水: %w", err)
		}
		if couponID != req.CouponID {
			return apperr.InvalidRequest.WithMessage("requestId 已用于其他优惠券")
		}
		if state == stateReleased {
			res.Already = true
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE cpn.coupon_stock SET remaining = remaining + 1 WHERE coupon_id = $1 AND bucket = $2`, couponID, int16(bucket)); err != nil {
			return fmt.Errorf("回补桶 %d: %w", bucket, err)
		}
		if _, err := tx.Exec(ctx, `UPDATE cpn.stock_ledger SET state = 'released', updated_at = now() WHERE request_id = $1`, req.RequestID); err != nil {
			return fmt.Errorf("标记流水已归还: %w", err)
		}
		res.Released = true
		return nil
	})
	if err != nil {
		return service.ReleaseResult{}, err
	}
	return res, nil
}

// startBucket 把 userId 经 murmur3 终结函数打散后对桶数取模：雪花 ID 的低位是递增序列号，
// 直接取模会让相邻用户挤在相邻桶上。
func startBucket(userID int64, buckets int) int {
	x := uint64(userID)
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return int(x % uint64(buckets))
}
