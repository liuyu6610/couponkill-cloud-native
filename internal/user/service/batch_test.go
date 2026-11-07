package service_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/apperr"
	"github.com/liuyu6610/couponkill-cloud-native/internal/user/service"
)

func gen(e *env, in service.BatchInput) (service.BatchResult, error) {
	return e.svc.GenerateBatch(context.Background(), in)
}

func TestBatchRejectsOutOfBoundsInput(t *testing.T) {
	tests := []struct {
		name string
		in   service.BatchInput
	}{
		{"count zero", service.BatchInput{Prefix: "lt", Count: 0}},
		{"count negative", service.BatchInput{Prefix: "lt", Count: -5}},
		{"count over max", service.BatchInput{Prefix: "lt", Count: service.MaxBatchCount + 1}},
		{"start negative", service.BatchInput{Prefix: "lt", Start: -1, Count: 1}},
		{"start over max", service.BatchInput{Prefix: "lt", Start: service.MaxBatchStart + 1, Count: 1}},
		{"empty prefix", service.BatchInput{Prefix: "", Count: 1}},
		{"prefix too long", service.BatchInput{Prefix: strings.Repeat("a", 33), Count: 1}},
		{"prefix with space", service.BatchInput{Prefix: "l t", Count: 1}},
		{"prefix with percent", service.BatchInput{Prefix: "l%", Count: 1}},
		{"prefix unicode", service.BatchInput{Prefix: "压测", Count: 1}},
		{"password too short", service.BatchInput{Prefix: "lt", Count: 1, Password: "abc"}},
		{"password too long", service.BatchInput{Prefix: "lt", Count: 1, Password: strings.Repeat("p", 73)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			_, err := gen(e, tc.in)
			if !errors.Is(err, apperr.InvalidRequest) {
				t.Fatalf("err = %v, want InvalidRequest", err)
			}
			if len(e.repo.batches) != 0 {
				t.Fatal("非法输入不得触达仓储")
			}
		})
	}
}

func TestBatchAcceptsBoundaryValues(t *testing.T) {
	e := newEnv(t)
	if _, err := gen(e, service.BatchInput{Prefix: strings.Repeat("a", 32), Start: service.MaxBatchStart, Count: 1}); err != nil {
		t.Fatalf("边界值应被接受: %v", err)
	}
}

func TestBatchCreatesPrefixedUsersWithValidShape(t *testing.T) {
	e := newEnv(t)
	res, err := gen(e, service.BatchInput{Prefix: "lt", Start: 5, Count: 3})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if res.Requested != 3 || res.Created != 3 || res.Existing != 0 {
		t.Fatalf("result = %+v", res)
	}
	phone := regexp.MustCompile(`^1[3-9][0-9]{9}$`)
	for _, name := range []string{"lt5", "lt6", "lt7"} {
		acc, err := e.repo.FindByUsername(context.Background(), name)
		if err != nil {
			t.Fatalf("%s not created: %v", name, err)
		}
		if acc.Role != "user" {
			t.Fatalf("%s role = %q, 压测用户必须是普通用户", name, acc.Role)
		}
		if acc.PasswordHash != "h:password123" {
			t.Fatalf("%s hash = %q, want 默认口令 password123 的哈希", name, acc.PasswordHash)
		}
	}
	for _, a := range e.repo.batches[0] {
		if !phone.MatchString(a.Phone) {
			t.Fatalf("phone %q 不符合注册规则", a.Phone)
		}
		if a.ID == 0 {
			t.Fatal("每个用户都要有应用生成的 ID")
		}
	}
}

func TestBatchHashesPasswordOnceNotPerUser(t *testing.T) {
	e := newEnv(t)
	if _, err := gen(e, service.BatchInput{Prefix: "lt", Count: 5000, Password: "loadtest1"}); err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if n := e.hasher.hashCalls.Load(); n != 1 {
		t.Fatalf("Hash 被调用 %d 次；bcrypt 逐用户计算 1 万人要数分钟，必须共用一个哈希", n)
	}
}

func TestBatchInsertsInBoundedChunks(t *testing.T) {
	e := newEnv(t)
	res, err := gen(e, service.BatchInput{Prefix: "lt", Count: 4500})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if res.Created != 4500 {
		t.Fatalf("created = %d", res.Created)
	}
	var sizes []int
	for _, b := range e.repo.batches {
		sizes = append(sizes, len(b))
		if len(b) > service.BatchChunkSize {
			t.Fatalf("批大小 %d 超过上限 %d", len(b), service.BatchChunkSize)
		}
	}
	if len(sizes) != 3 || sizes[0] != 2000 || sizes[1] != 2000 || sizes[2] != 500 {
		t.Fatalf("chunk sizes = %v, want [2000 2000 500]", sizes)
	}
}

func TestBatchIsIdempotentOnRerunAndOverlap(t *testing.T) {
	e := newEnv(t)
	in := service.BatchInput{Prefix: "lt", Start: 0, Count: 100}
	first, err := gen(e, in)
	if err != nil || first.Created != 100 {
		t.Fatalf("first = %+v %v", first, err)
	}
	second, err := gen(e, in)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if second.Created != 0 || second.Existing != 100 || second.Requested != 100 {
		t.Fatalf("重跑应是空操作: %+v", second)
	}
	overlap, err := gen(e, service.BatchInput{Prefix: "lt", Start: 50, Count: 100})
	if err != nil {
		t.Fatalf("overlap: %v", err)
	}
	if overlap.Created != 50 || overlap.Existing != 50 {
		t.Fatalf("重叠区间只应新建 50 个: %+v", overlap)
	}
}

func TestBatchNeverOverwritesExistingUser(t *testing.T) {
	e := newEnv(t)
	e.repo.seed("lt1", "h:original", "admin", service.StatusActive, 99)
	if _, err := gen(e, service.BatchInput{Prefix: "lt", Start: 0, Count: 3}); err != nil {
		t.Fatal(err)
	}
	acc, _ := e.repo.FindByUsername(context.Background(), "lt1")
	if acc.PasswordHash != "h:original" || acc.Role != "admin" || acc.ID != 99 {
		t.Fatalf("已有用户被改动: %+v", acc)
	}
}

func TestBatchStopsWhenContextCancelled(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.svc.GenerateBatch(ctx, service.BatchInput{Prefix: "lt", Count: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(e.repo.batches) != 0 {
		t.Fatal("ctx 已取消就不应再插入")
	}
}

func TestBatchRepoFailureAbortsAndIsSystemError(t *testing.T) {
	e := newEnv(t)
	e.repo.failNext = errBoom
	_, err := gen(e, service.BatchInput{Prefix: "lt", Count: 5})
	if !errors.Is(err, errBoom) || apperr.Resolve(err).Code != apperr.CodeSysError {
		t.Fatalf("err = %v", err)
	}
}
