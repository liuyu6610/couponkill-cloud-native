package dbx

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE 码（https://www.postgresql.org/docs/current/errcodes-appendix.html）。
const (
	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
	sqlStateCheckViolation      = "23514"
	sqlStateSerializationFail   = "40001"
	sqlStateDeadlockDetected    = "40P01"
)

// pgErrorWithCode 在错误链中查找 *pgconn.PgError，且 SQLSTATE 等于 code、
// 约束名匹配（constraint 为空表示不限约束）时返回 true。
func pgErrorWithCode(err error, code, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// IsUniqueViolation 判断 err 是否为唯一约束/唯一索引冲突（SQLSTATE 23505）。
// constraint 非空时还要求冲突的约束（或索引）名等于它；为空则匹配任意唯一冲突。
func IsUniqueViolation(err error, constraint string) bool {
	return pgErrorWithCode(err, sqlStateUniqueViolation, constraint)
}

// IsForeignKeyViolation 判断 err 是否为外键冲突（SQLSTATE 23503），constraint 语义同 IsUniqueViolation。
func IsForeignKeyViolation(err error, constraint string) bool {
	return pgErrorWithCode(err, sqlStateForeignKeyViolation, constraint)
}

// IsCheckViolation 判断 err 是否为 CHECK 约束冲突（SQLSTATE 23514），constraint 语义同 IsUniqueViolation。
func IsCheckViolation(err error, constraint string) bool {
	return pgErrorWithCode(err, sqlStateCheckViolation, constraint)
}

// IsNoRows 判断 err 是否表示“查询无结果”（pgx.ErrNoRows）。
func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// IsSerializationFailure 判断 err 是否为事务序列化失败（SQLSTATE 40001）。
func IsSerializationFailure(err error) bool {
	return pgErrorWithCode(err, sqlStateSerializationFail, "")
}

// IsDeadlock 判断 err 是否为死锁被检测并中止（SQLSTATE 40P01）。
func IsDeadlock(err error) bool {
	return pgErrorWithCode(err, sqlStateDeadlockDetected, "")
}

// IsRetryable 判断整个事务重试是否可能成功：序列化失败或死锁。
func IsRetryable(err error) bool {
	return IsSerializationFailure(err) || IsDeadlock(err)
}
