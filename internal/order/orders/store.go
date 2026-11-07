package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// 订单状态，取值与 ord.orders.status、前端 types/api.ts 一致。
const (
	// StatusCreated 是已创建（持有中）。
	StatusCreated = 1
	// StatusUsed 是已使用。
	StatusUsed = 2
	// StatusExpired 是已过期。
	StatusExpired = 3
	// StatusCancelled 是已取消。
	StatusCancelled = 4
)

// 订单来源，取值与 ord.orders.source 的 CHECK 一致。
const (
	// SourceSeckill 是秒杀流水线落的单（含预约代抢）。
	SourceSeckill = "seckill"
	// SourceNormal 是常驻券 /order/create 落的单。
	SourceNormal = "normal"
	// SourceReservation 预留给预约代抢直接建单的场景。
	SourceReservation = "reservation"
	// SourceAdmin 是管理员发券落的单。
	SourceAdmin = "admin"
)

// 券类型，与 ord.orders.coupon_type 一致。
const (
	// CouponTypeNormal 是常驻券。
	CouponTypeNormal = 1
	// CouponTypeSeckill 是秒杀券。
	CouponTypeSeckill = 2
)

// Querier 是 store 需要的最小数据库能力；*pgxpool.Pool 与 pgx.Tx 都满足它，
// 因此同一组函数既能单独执行，也能放进调用方的事务里。
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Order 是订单行及其对外 JSON 形状（ID 一律为字符串，金额保持两位小数原样文本）。
type Order struct {
	// ID 是订单号（snowflake）。
	ID int64 `json:"id,string"`
	// UserID 是持有人。
	UserID int64 `json:"userId,string"`
	// CouponID 是券 ID。
	CouponID int64 `json:"couponId,string"`
	// RequestID 是秒杀/发券/下单的请求号，也是库存账本的幂等键。
	RequestID string `json:"requestId"`
	// CouponName 是下单时拷贝的券名称。
	CouponName string `json:"couponName"`
	// CouponType 是下单时拷贝的券类型，见 CouponType*。
	CouponType int `json:"couponType"`
	// FaceValue 是面额快照。
	FaceValue json.Number `json:"faceValue"`
	// MinSpend 是使用门槛快照。
	MinSpend json.Number `json:"minSpend"`
	// Status 是订单状态，见 Status*。
	Status int `json:"status"`
	// Source 是订单来源，见 Source*。
	Source string `json:"source"`
	// GetTime 是领取时间。
	GetTime time.Time `json:"getTime"`
	// ExpireTime 是过期时间。
	ExpireTime time.Time `json:"expireTime"`
	// UseTime 是使用时间，未使用为空。
	UseTime *time.Time `json:"useTime,omitempty"`
	// CancelTime 是取消时间，未取消为空。
	CancelTime *time.Time `json:"cancelTime,omitempty"`
	// CreatedAt 是行创建时间。
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt 是行更新时间。
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewOrder 是要插入的订单；ID 与时间由调用方给出，便于测试注入时钟与 ID 源。
type NewOrder struct {
	// ID 是预分配的订单号。
	ID int64
	// UserID 是持有人。
	UserID int64
	// CouponID 是券 ID。
	CouponID int64
	// RequestID 是幂等键，(UserID, RequestID) 唯一。
	RequestID string
	// CouponName 是券名称快照。
	CouponName string
	// CouponType 是券类型快照。
	CouponType int
	// FaceValue 是面额快照（十进制文本，空串按 0 处理）。
	FaceValue string
	// MinSpend 是使用门槛快照（十进制文本，空串按 0 处理）。
	MinSpend string
	// Source 是订单来源，见 Source*。
	Source string
	// Now 是领取时间。
	Now time.Time
	// ExpireAt 是过期时间，必须晚于 Now。
	ExpireAt time.Time
}

const orderColumns = `id, user_id, coupon_id, request_id, coupon_name, coupon_type,
	face_value::text, min_spend::text, status, source, get_time, expire_time, use_time, cancel_time, created_at, updated_at`

const insertOrderSQL = `
INSERT INTO ord.orders (id, user_id, coupon_id, request_id, coupon_name, coupon_type,
	face_value, min_spend, status, source, get_time, expire_time, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7::text::numeric, $8::text::numeric, 1, $9, $10, $11, $10, $10)
ON CONFLICT (user_id, request_id) DO NOTHING
RETURNING id`

// Insert 插入订单，并通过 ON CONFLICT (user_id, request_id) DO NOTHING 吸收同一请求号的重放：
// 返回 inserted=false 表示该请求号此前已落过单（再用 FindByRequest 取得已有订单）。
// 违反 (user_id, coupon_id) 部分唯一索引时返回的错误满足 dbx.IsUniqueViolation(err, "")，
// 调用方应把它解释为「用户已持有该券」；注意不要按约束名匹配，分区表报告的是分区索引名。
func Insert(ctx context.Context, q Querier, o NewOrder) (id int64, inserted bool, err error) {
	face, spend := o.FaceValue, o.MinSpend
	if face == "" {
		face = "0"
	}
	if spend == "" {
		spend = "0"
	}
	err = q.QueryRow(ctx, insertOrderSQL, o.ID, o.UserID, o.CouponID, o.RequestID, o.CouponName, o.CouponType,
		face, spend, o.Source, o.Now, o.ExpireAt).Scan(&id)
	switch {
	case err == nil:
		return id, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("insert order: %w", err)
	}
}

// InsertNotification 在订单所在事务里写一条站内通知；读取由 notification 模块负责。
func InsertNotification(ctx context.Context, q Querier, id, userID int64, typ, title, content string, refID int64, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO ord.notifications (id, user_id, type, title, content, ref_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, userID, typ, title, content, refID, now)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var face, spend string
	err := row.Scan(&o.ID, &o.UserID, &o.CouponID, &o.RequestID, &o.CouponName, &o.CouponType,
		&face, &spend, &o.Status, &o.Source, &o.GetTime, &o.ExpireTime, &o.UseTime, &o.CancelTime, &o.CreatedAt, &o.UpdatedAt)
	o.FaceValue, o.MinSpend = json.Number(face), json.Number(spend)
	return o, err
}

func collectOrders(rows pgx.Rows) ([]Order, error) {
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read orders: %w", err)
	}
	return out, nil
}

// FindByRequest 按 (user_id, request_id) 查订单（带分区键，命中唯一索引）。
func FindByRequest(ctx context.Context, q Querier, userID int64, requestID string) (Order, bool, error) {
	o, err := scanOrder(q.QueryRow(ctx, `SELECT `+orderColumns+` FROM ord.orders WHERE user_id = $1 AND request_id = $2`, userID, requestID))
	return foundOrder(o, err, "find order by request")
}

// FindByID 按 (user_id, id) 查订单（带分区键，只扫一个分区）。
func FindByID(ctx context.Context, q Querier, userID, id int64) (Order, bool, error) {
	o, err := scanOrder(q.QueryRow(ctx, `SELECT `+orderColumns+` FROM ord.orders WHERE user_id = $1 AND id = $2`, userID, id))
	return foundOrder(o, err, "find order by id")
}

func foundOrder(o Order, err error, what string) (Order, bool, error) {
	switch {
	case err == nil:
		return o, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return Order{}, false, nil
	default:
		return Order{}, false, fmt.Errorf("%s: %w", what, err)
	}
}

// Cancel 把订单从 1（已创建）置为 4（已取消），并返回是否由本次调用完成。
// 并发取消只有一个调用能得到 true：其余看到 status 已不是 1，更新 0 行。
func Cancel(ctx context.Context, q Querier, userID, id int64, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE ord.orders SET status = 4, cancel_time = $3, updated_at = $3
WHERE user_id = $1 AND id = $2 AND status = 1`, userID, id, now)
	if err != nil {
		return false, fmt.Errorf("cancel order: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// CancelByRequest 与 Cancel 相同，但按 (user_id, request_id) 定位订单；
// 用于回收「请求已终态失败、订单却迟到落库」的孤儿订单。
func CancelByRequest(ctx context.Context, q Querier, userID int64, requestID string, now time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE ord.orders SET status = 4, cancel_time = $3, updated_at = $3
WHERE user_id = $1 AND request_id = $2 AND status = 1`, userID, requestID, now)
	if err != nil {
		return false, fmt.Errorf("cancel order by request: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// HasActive 判断用户是否持有该券的未取消订单（口径与部分唯一索引一致：status <> 4）。
func HasActive(ctx context.Context, q Querier, userID, couponID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ord.orders WHERE user_id = $1 AND coupon_id = $2 AND status <> 4)`,
		userID, couponID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check active order: %w", err)
	}
	return ok, nil
}

// Counts 是用户持有的券统计：Total 为持有中（已创建 + 已使用），Seckill/Normal 按券类型细分，Expired 为已过期。
type Counts struct {
	// Total 是持有的券总数（status 1、2）。
	Total int `json:"totalCount"`
	// Seckill 是其中的秒杀券数。
	Seckill int `json:"seckillCount"`
	// Normal 是其中的常驻券数。
	Normal int `json:"normalCount"`
	// Expired 是已过期的券数（status 3）。
	Expired int `json:"expiredCount"`
}

// CountsByUser 聚合用户的券统计（带 user_id，分区裁剪）。
func CountsByUser(ctx context.Context, q Querier, userID int64) (Counts, error) {
	var c Counts
	err := q.QueryRow(ctx, `SELECT
	count(*) FILTER (WHERE status IN (1, 2)),
	count(*) FILTER (WHERE status IN (1, 2) AND coupon_type = 2),
	count(*) FILTER (WHERE status IN (1, 2) AND coupon_type = 1),
	count(*) FILTER (WHERE status = 3)
FROM ord.orders WHERE user_id = $1`, userID).Scan(&c.Total, &c.Seckill, &c.Normal, &c.Expired)
	if err != nil {
		return Counts{}, fmt.Errorf("count user orders: %w", err)
	}
	return c, nil
}

// ListByUser 返回用户订单（创建时间倒序）与总数，offset/limit 由调用方保证非负且有上限。
func ListByUser(ctx context.Context, q Querier, userID int64, offset, limit int) ([]Order, int64, error) {
	var total int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM ord.orders WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count user orders: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT `+orderColumns+` FROM ord.orders WHERE user_id = $1
ORDER BY created_at DESC, id DESC OFFSET $2 LIMIT $3`, userID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list user orders: %w", err)
	}
	items, err := collectOrders(rows)
	return items, total, err
}

// AdminFilter 是管理端列表的过滤条件；零值字段不过滤。
type AdminFilter struct {
	// OrderID 按订单号点查（只能靠 idx_orders_id，会扫描分区）。
	OrderID int64
	// UserID 限定用户（命中分区裁剪）。
	UserID int64
	// CouponID 限定券。
	CouponID int64
	// Status 限定状态，0 表示不限。
	Status int
	// From 限定创建时间下界（含）。
	From *time.Time
	// To 限定创建时间上界（含）。
	To *time.Time
}

func (f AdminFilter) where() (string, []any) {
	var conds []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.OrderID > 0 {
		add("id = $%d", f.OrderID)
	}
	if f.UserID > 0 {
		add("user_id = $%d", f.UserID)
	}
	if f.CouponID > 0 {
		add("coupon_id = $%d", f.CouponID)
	}
	if f.Status > 0 {
		add("status = $%d", f.Status)
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at <= $%d", *f.To)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListAll 是管理端列表：可能跨全部分区，调用方必须限制 offset 与 limit。
func ListAll(ctx context.Context, q Querier, f AdminFilter, offset, limit int) ([]Order, int64, error) {
	where, args := f.where()
	var total int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM ord.orders`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}
	args = append(args, offset, limit)
	rows, err := q.Query(ctx, fmt.Sprintf(`SELECT `+orderColumns+` FROM ord.orders%s
ORDER BY created_at DESC, id DESC OFFSET $%d LIMIT $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	items, err := collectOrders(rows)
	return items, total, err
}
