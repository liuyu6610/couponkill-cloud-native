package notification

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
)

func TestInsertValidatesAndStoresFields(t *testing.T) {
	_, pool := newService(t)
	ctx := context.Background()

	bad := []Notification{
		{ID: 0, UserID: 1, Type: "T", Title: "t"},
		{ID: 1, UserID: 0, Type: "T", Title: "t"},
		{ID: 1, UserID: 1, Type: "", Title: "t"},
		{ID: 1, UserID: 1, Type: "T", Title: ""},
	}
	for i, n := range bad {
		if err := Insert(ctx, pool, n); err == nil {
			t.Errorf("非法通知 #%d 应被拒绝: %+v", i, n)
		}
	}

	ref := int64(77)
	if err := Insert(ctx, pool, Notification{
		ID: 5, UserID: 9, Type: TypeReservationFailed, Title: "预约帮抢失败", Content: "券 1 履约失败", RefID: &ref,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	svc := NewService(pool)
	items, total, err := svc.List(ctx, 9, 1, 20)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("List = %v total=%d err=%v", items, total, err)
	}
	got := items[0]
	if got.ID != 5 || got.UserID != 9 || got.Type != TypeReservationFailed || got.Title != "预约帮抢失败" ||
		got.Content != "券 1 履约失败" || got.RefID == nil || *got.RefID != 77 || got.ReadFlag || got.CreatedAt.IsZero() {
		t.Fatalf("字段不符: %+v", got)
	}
}

func TestListIsNewestFirstPagedAndScopedToUser(t *testing.T) {
	svc, pool := newService(t)
	ids := &seqIDs{}
	mine := seed(t, pool, 1, ids, 5)
	seed(t, pool, 2, ids, 3) // 别人的通知不得出现
	ctx := context.Background()

	items, total, err := svc.List(ctx, 1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(items) != 2 || items[0].ID != mine[4] || items[1].ID != mine[3] {
		t.Fatalf("第 1 页应为最新两条: total=%d items=%+v", total, items)
	}
	items, _, err = svc.List(ctx, 1, 3, 2)
	if err != nil || len(items) != 1 || items[0].ID != mine[0] {
		t.Fatalf("第 3 页应只剩最老一条: %+v err=%v", items, err)
	}
	items, total, err = svc.List(ctx, 1, 4, 2)
	if err != nil || len(items) != 0 || total != 5 {
		t.Fatalf("越界页应为空且 total 不变: %+v total=%d err=%v", items, total, err)
	}
	items, total, err = svc.List(ctx, 42, 1, 20)
	if err != nil || len(items) != 0 || total != 0 {
		t.Fatalf("无通知的用户: %+v total=%d err=%v", items, total, err)
	}
}

func TestUnreadCountTracksReadAndReadAll(t *testing.T) {
	svc, pool := newService(t)
	ids := &seqIDs{}
	ours := seed(t, pool, 1, ids, 4)
	seed(t, pool, 2, ids, 2)
	ctx := context.Background()

	count := func(user int64) int64 {
		t.Helper()
		n, err := svc.UnreadCount(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(1) != 4 || count(2) != 2 {
		t.Fatalf("初始未读数应为 4/2, 得到 %d/%d", count(1), count(2))
	}
	if err := svc.MarkRead(ctx, 1, ours[0]); err != nil {
		t.Fatal(err)
	}
	if count(1) != 3 {
		t.Fatalf("读一条后应剩 3, 得到 %d", count(1))
	}
	// 重复标记同一条已读：幂等，未读数不再变
	if err := svc.MarkRead(ctx, 1, ours[0]); err != nil {
		t.Fatalf("重复标记已读应成功: %v", err)
	}
	if count(1) != 3 {
		t.Fatalf("重复已读不应改变未读数, 得到 %d", count(1))
	}
	n, err := svc.MarkAllRead(ctx, 1)
	if err != nil || n != 3 {
		t.Fatalf("MarkAllRead 应返回新标记的 3 条: n=%d err=%v", n, err)
	}
	if count(1) != 0 || count(2) != 2 {
		t.Fatalf("全部已读只影响自己: %d/%d", count(1), count(2))
	}
	n, err = svc.MarkAllRead(ctx, 1)
	if err != nil || n != 0 {
		t.Fatalf("二次全部已读应返回 0: n=%d err=%v", n, err)
	}
}

func TestMarkReadEnforcesOwnership(t *testing.T) {
	svc, pool := newService(t)
	ids := &seqIDs{}
	theirs := seed(t, pool, 2, ids, 1)
	ctx := context.Background()

	for name, id := range map[string]int64{"别人的通知": theirs[0], "不存在的通知": 999999} {
		err := svc.MarkRead(ctx, 1, id)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.HTTPStatus != http.StatusNotFound {
			t.Errorf("%s应返回 404, 得到 %v", name, err)
		}
	}
	// 别人的通知不能被我读掉
	n, err := svc.UnreadCount(ctx, 2)
	if err != nil || n != 1 {
		t.Fatalf("用户 2 的通知应仍未读: n=%d err=%v", n, err)
	}
}

func TestInsertWorksInsideTransactionAndRollsBack(t *testing.T) {
	svc, pool := newService(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := Insert(ctx, tx, Notification{ID: 1, UserID: 1, Type: TypeSeckillSuccess, Title: "秒杀成功"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.UnreadCount(ctx, 1); err != nil || n != 0 {
		t.Fatalf("回滚后不应留下通知: n=%d err=%v", n, err)
	}
}
