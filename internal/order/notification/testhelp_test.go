package notification

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/liuyu6610/couponkill-cloud-native/internal/order/migrations"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/dbx/dbxtest"
)

// seqIDs 是测试用的单调递增 ID 生成器。
type seqIDs struct{ n atomic.Int64 }

func (s *seqIDs) Next() int64 { return s.n.Add(1) }

// newService 返回绑定到隔离数据库（已应用 ord 迁移）的 Service 与连接池。
func newService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := dbxtest.New(t)
	if err := dbx.Migrate(context.Background(), pool, "ord", migrations.FS); err != nil {
		t.Fatalf("迁移 ord schema: %v", err)
	}
	return NewService(pool), pool
}

// seed 给 userID 插入 n 条通知，创建时间依次相差 1 分钟（后插入的更新），返回按插入顺序的 ID。
func seed(t *testing.T, pool *pgxpool.Pool, userID int64, ids *seqIDs, n int) []int64 {
	t.Helper()
	base := time.Now().Add(-time.Hour).UTC()
	out := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		id := ids.Next()
		err := Insert(context.Background(), pool, Notification{
			ID: id, UserID: userID, Type: TypeReservationSuccess, Title: "标题", Content: "内容",
			CreatedAt: base.Add(time.Duration(id) * time.Minute),
		})
		if err != nil {
			t.Fatalf("插入通知: %v", err)
		}
		out = append(out, id)
	}
	return out
}
