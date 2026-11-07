// Package migrations 内嵌 ord schema 的迁移 SQL（NNNN_name.sql）。
package migrations

import "embed"

// FS 包含本目录下所有 .sql 文件，由 dbx.Migrate 按版本顺序执行。
//
//go:embed *.sql
var FS embed.FS
