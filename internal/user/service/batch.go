package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/authx"
	"github.com/liuyu6610/couponkill-cloud-native/internal/platform/logx"
)

// 压测造用户的边界。
const (
	// MaxBatchCount 是单次请求最多创建的用户数（沿用 Java 的 100000 上限）。
	MaxBatchCount = 100000
	// MaxBatchStart 是起始序号上限，保证 前缀(≤32)+序号(≤10 位) 不超过用户名 50 字符的限制。
	MaxBatchStart = 1_000_000_000
	// BatchChunkSize 是每条 INSERT 的行数上限，限制单个语句与内存占用。
	BatchChunkSize = 2000
	// DefaultBatchPassword 是未指定口令时压测用户的口令（沿用 Java 的 password123）。
	DefaultBatchPassword = "password123"
)

var batchPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// BatchInput 描述一次造用户请求：创建用户名 Prefix+Start … Prefix+(Start+Count-1)。
type BatchInput struct {
	// Prefix 是用户名前缀，字母数字下划线连字符，1..32 个字符。
	Prefix string
	// Start 是起始序号，0..MaxBatchStart。
	Start int
	// Count 是数量，1..MaxBatchCount。
	Count int
	// Password 是这批用户共用的口令，空串取 DefaultBatchPassword，其余规则同注册。
	Password string
}

// BatchResult 是造用户的结果。
type BatchResult struct {
	// Requested 是请求的数量。
	Requested int
	// Created 是本次实际新建的数量。
	Created int
	// Existing 是因用户名已存在而跳过的数量。
	Existing int
}

// GenerateBatch 为压测批量创建普通用户，幂等且可重叠重跑：已存在的用户名被跳过，绝不覆盖。
//
// 口令只做一次 bcrypt，整批共用同一个哈希（逐用户 bcrypt cost 10 造 1 万人需要数分钟 CPU，
// 压测用户的口令本来就是公开的测试值）。按 BatchChunkSize 分块插入，块与块之间检查 ctx，
// 中途失败时已插入的块保留，重跑即可补齐。
func (s *Service) GenerateBatch(ctx context.Context, in BatchInput) (BatchResult, error) {
	if err := validateBatch(&in); err != nil {
		return BatchResult{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return BatchResult{}, fmt.Errorf("hash batch password: %w", err)
	}

	created := 0
	for offset := 0; offset < in.Count; offset += BatchChunkSize {
		if err := ctx.Err(); err != nil {
			return BatchResult{}, fmt.Errorf("batch generate interrupted after %d users: %w", created, err)
		}
		n := min(BatchChunkSize, in.Count-offset)
		chunk := make([]NewAccount, n)
		for i := range chunk {
			seq := in.Start + offset + i
			name := in.Prefix + strconv.Itoa(seq)
			chunk[i] = NewAccount{
				ID:           s.ids.Next(),
				Username:     name,
				PasswordHash: hash,
				Phone:        fmt.Sprintf("1380000%04d", seq%10000),
				Email:        name + "@loadtest.local",
				Role:         authx.RoleUser,
			}
		}
		inserted, err := s.repo.InsertBatch(ctx, chunk)
		if err != nil {
			return BatchResult{}, fmt.Errorf("insert batch chunk at offset %d (created so far %d): %w", offset, created, err)
		}
		created += inserted
	}

	res := BatchResult{Requested: in.Count, Created: created, Existing: in.Count - created}
	logx.From(ctx).Info("批量造用户完成", "prefix", in.Prefix, "start", in.Start,
		"requested", res.Requested, "created", res.Created, "existing", res.Existing)
	return res, nil
}

// validateBatch 校验边界并补上默认口令。
func validateBatch(in *BatchInput) error {
	if !batchPrefixPattern.MatchString(in.Prefix) {
		return invalid("prefix 必须是 1..32 位字母、数字、下划线或连字符")
	}
	if in.Count < 1 || in.Count > MaxBatchCount {
		return invalid("count 必须在 1 到 100000 之间")
	}
	if in.Start < 0 || in.Start > MaxBatchStart {
		return invalid("start 必须在 0 到 1000000000 之间")
	}
	if in.Password == "" {
		in.Password = DefaultBatchPassword
	}
	return validateNewPassword(in.Password)
}
