package dbx

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func pgErr(code, constraint string) error {
	return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code, ConstraintName: constraint})
}

func TestIsUniqueViolation(t *testing.T) {
	err := pgErr("23505", "uk_orders_user_request")
	if !IsUniqueViolation(err, "uk_orders_user_request") {
		t.Fatal("同名约束应判定为唯一冲突")
	}
	if !IsUniqueViolation(err, "") {
		t.Fatal("约束名为空应匹配任意唯一冲突")
	}
	if IsUniqueViolation(err, "other") {
		t.Fatal("约束名不同不应匹配")
	}
	if IsUniqueViolation(pgErr("23503", "uk_orders_user_request"), "") {
		t.Fatal("外键冲突不是唯一冲突")
	}
	if IsUniqueViolation(nil, "") || IsUniqueViolation(errors.New("x"), "") {
		t.Fatal("非 PgError 不应匹配")
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	err := pgErr("23503", "fk_a")
	if !IsForeignKeyViolation(err, "fk_a") || !IsForeignKeyViolation(err, "") {
		t.Fatal("应判定为外键冲突")
	}
	if IsForeignKeyViolation(err, "fk_b") {
		t.Fatal("约束名不同不应匹配")
	}
	if IsForeignKeyViolation(pgErr("23505", "fk_a"), "") {
		t.Fatal("唯一冲突不是外键冲突")
	}
}

func TestIsCheckViolation(t *testing.T) {
	if !IsCheckViolation(pgErr("23514", "ck_remaining"), "ck_remaining") {
		t.Fatal("应判定为 CHECK 冲突")
	}
	if IsCheckViolation(pgErr("23514", "ck_remaining"), "x") {
		t.Fatal("约束名不同不应匹配")
	}
}

func TestIsNoRows(t *testing.T) {
	if !IsNoRows(fmt.Errorf("get user: %w", pgx.ErrNoRows)) {
		t.Fatal("包装后的 ErrNoRows 应被识别")
	}
	if IsNoRows(errors.New("boom")) || IsNoRows(nil) {
		t.Fatal("其他错误不应被识别")
	}
}

func TestRetryableClassification(t *testing.T) {
	if !IsSerializationFailure(pgErr("40001", "")) {
		t.Fatal("40001 是序列化失败")
	}
	if !IsDeadlock(pgErr("40P01", "")) {
		t.Fatal("40P01 是死锁")
	}
	if !IsRetryable(pgErr("40001", "")) || !IsRetryable(pgErr("40P01", "")) {
		t.Fatal("两者都应可重试")
	}
	if IsRetryable(pgErr("23505", "")) || IsRetryable(errors.New("x")) {
		t.Fatal("唯一冲突与普通错误不可重试")
	}
}
