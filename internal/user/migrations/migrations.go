// Package migrations 内嵌 usr schema 的迁移 SQL（NNNN_name.sql）与演示种子（seed_demo.sql）。
package migrations

import "embed"

// FS 包含本目录下所有 .sql 文件：NNNN_name.sql 由 dbx.Migrate 按版本顺序执行；
// seed_demo.sql 不是迁移，只在 CK_SEED_DEMO=true 时由服务显式执行（读取 SeedDemoFile）。
//
//go:embed *.sql
var FS embed.FS

// SeedDemoFile 是演示种子文件名，用 FS.ReadFile(SeedDemoFile) 读取后在 CK_SEED_DEMO=true 时执行。
const SeedDemoFile = "seed_demo.sql"
