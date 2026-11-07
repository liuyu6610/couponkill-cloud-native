package repo_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/testkit"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/repo"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

// touch 以 service 使用的固定间隔（1 小时）调用 TouchLastActive。
func touch(r *repo.Repo, ctx context.Context, id int64, now time.Time) (bool, error) {
	return r.TouchLastActive(ctx, id, now, service.LastActiveRefreshInterval)
}

// counterIDs 是自增 ID 生成器，起点避开种子用户的 ID。
type counterIDs struct{ n atomic.Int64 }

func (c *counterIDs) Next() int64 { return c.n.Add(1) + 1_000_000 }

// newService 用真实仓储和真实 bcrypt 构造 service。
func newService(t *testing.T, r *repo.Repo) *service.Service {
	t.Helper()
	tokens, err := authx.NewTokens(authx.TokenConfig{
		Secret: testkit.JWTSecret, Issuer: "couponkill", Audience: "couponkill-api", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	svc, err := service.New(service.Deps{Repo: r, Sessions: authx.NewMemoryRevoker(), Tokens: tokens, IDs: &counterIDs{}})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}
