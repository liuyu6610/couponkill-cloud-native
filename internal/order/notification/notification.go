// Package notification 实现 order 服务的站内通知：ord.notifications 表的读写与 HTTP 接口。
//
// 写入方有两个：本服务的预约流程（reservation 包）与秒杀建单（order-core，类型
// TypeSeckillSuccess，自己在建单事务里写）。两者都通过 Insert 或同样的列写入同一张表。
// 读取方是前端的铃铛：列表、未读数、单条已读、全部已读，全部限定为调用者本人。
//
// 不变式：
//  1. 所有读写都带 user_id 谓词（SQL 层强制属主，且命中 (user_id, ...) 索引）；
//     访问别人的通知与访问不存在的通知不可区分，一律 404。
//  2. 已读标记幂等：重复标记同一条返回成功，全部已读返回“本次新标记”的条数（再调为 0）。
package notification

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

// 通知类型词表（ord.notifications.type）。
const (
	// TypeSeckillSuccess 是秒杀建单成功，由 order-core 在建单事务里写入。
	TypeSeckillSuccess = "SECKILL_SUCCESS"
	// TypeReservationSuccess 是预约帮抢成功。
	TypeReservationSuccess = "RESERVATION_SUCCESS"
	// TypeReservationFailed 是预约帮抢失败（含重试耗尽、重复秒杀、库存不足等终态失败）。
	TypeReservationFailed = "RESERVATION_FAILED"
	// TypeReservationExpired 是预约过期（活动结束仍未能触发）。
	TypeReservationExpired = "RESERVATION_EXPIRED"
)

// ErrNotFound 表示通知不存在或不属于调用者（对外同一个 404，避免泄露他人通知的存在）。
var ErrNotFound = apperr.New(apperr.CodeInvalidRequest, http.StatusNotFound, "通知不存在")

// DBTX 是 Insert 与 Service 需要的最小数据库能力；*pgxpool.Pool 与 pgx.Tx 都满足它，
// 因此调用方可以把通知写进自己的事务里。
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Notification 是一条站内通知，JSON 字段名与前端 UserNotification 类型一致（ID 一律输出为字符串）。
type Notification struct {
	// ID 是雪花 ID，由调用方生成。
	ID int64 `json:"id,string"`
	// UserID 是收件人。
	UserID int64 `json:"userId,string"`
	// Type 见 Type* 常量。
	Type string `json:"type"`
	// Title 是标题，必填。
	Title string `json:"title"`
	// Content 是正文，可空。
	Content string `json:"content"`
	// RefID 是关联对象（如预约 ID），无关联为 nil。
	RefID *int64 `json:"refId,string"`
	// ReadFlag 是否已读。
	ReadFlag bool `json:"readFlag"`
	// CreatedAt 是创建时间；Insert 时为零值则由数据库取当前时间。
	CreatedAt time.Time `json:"createdAt"`
}

const columns = "id, user_id, type, title, content, ref_id, read_flag, created_at"

// Insert 写入一条通知。ID、UserID、Type、Title 必填；db 可以是连接池或事务。
// 同一 ID 重复写入会返回主键冲突错误（调用方应保证 ID 唯一，幂等性由调用方的状态迁移保证）。
func Insert(ctx context.Context, db DBTX, n Notification) error {
	switch {
	case n.ID <= 0:
		return errors.New("通知 ID 必须为正数")
	case n.UserID <= 0:
		return errors.New("通知收件人必须为正数")
	case n.Type == "":
		return errors.New("通知类型不能为空")
	case n.Title == "":
		return errors.New("通知标题不能为空")
	}
	var created *time.Time
	if !n.CreatedAt.IsZero() {
		created = &n.CreatedAt
	}
	_, err := db.Exec(ctx,
		`INSERT INTO ord.notifications (id, user_id, type, title, content, ref_id, read_flag, created_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, false, COALESCE($7, now()))`,
		n.ID, n.UserID, n.Type, n.Title, n.Content, n.RefID, created)
	if err != nil {
		return fmt.Errorf("写入通知: %w", err)
	}
	return nil
}

// Service 提供调用者本人维度的通知读写。并发安全。
type Service struct {
	db DBTX
}

// NewService 创建 Service。
func NewService(db DBTX) *Service { return &Service{db: db} }

// List 返回 userID 的通知，最新的在前；page 从 1 开始。total 是该用户的通知总数。
func (s *Service) List(ctx context.Context, userID int64, page, size int) ([]Notification, int64, error) {
	var total int64
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM ord.notifications WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计通知: %w", err)
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+columns+` FROM ord.notifications WHERE user_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`,
		userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("查询通知: %w", err)
	}
	defer rows.Close()
	items := make([]Notification, 0, size)
	for rows.Next() {
		var n Notification
		var content *string
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &content, &n.RefID, &n.ReadFlag, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("读取通知: %w", err)
		}
		if content != nil {
			n.Content = *content
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历通知: %w", err)
	}
	return items, total, nil
}

// UnreadCount 返回 userID 的未读通知数（命中部分索引 idx_notifications_unread）。
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM ord.notifications WHERE user_id = $1 AND NOT read_flag`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计未读通知: %w", err)
	}
	return n, nil
}

// MarkRead 把 userID 的一条通知标为已读。通知不存在或不属于该用户返回 ErrNotFound；
// 已经是已读时同样成功（幂等）。
func (s *Service) MarkRead(ctx context.Context, userID, id int64) error {
	var found int
	err := s.db.QueryRow(ctx,
		`UPDATE ord.notifications SET read_flag = true WHERE user_id = $1 AND id = $2 RETURNING 1`,
		userID, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("标记通知已读: %w", err)
	}
	return nil
}

// MarkAllRead 把 userID 的全部未读通知标为已读，返回本次新标记的条数。
func (s *Service) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`UPDATE ord.notifications SET read_flag = true WHERE user_id = $1 AND NOT read_flag`, userID)
	if err != nil {
		return 0, fmt.Errorf("全部标记已读: %w", err)
	}
	return tag.RowsAffected(), nil
}
