package dbx

import (
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsOrdersByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0010_c.sql": {Data: []byte("SELECT 3;")},
		"0001_a.sql": {Data: []byte("SELECT 1;")},
		"0002_b.sql": {Data: []byte("SELECT 2;")},
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 3 || migs[0].version != 1 || migs[1].version != 2 || migs[2].version != 10 {
		t.Fatalf("应按版本号数值升序: %+v", migs)
	}
	if migs[0].name != "a" {
		t.Fatalf("name 应取自文件名: %q", migs[0].name)
	}
}

func TestLoadMigrationsIgnoresNonMigrationFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_a.sql":     {Data: []byte("SELECT 1;")},
		"seed_demo.sql":  {Data: []byte("INSERT ...")},
		"migrations.go":  {Data: []byte("package migrations")},
		"README.md":      {Data: []byte("x")},
		"sub/0002_x.sql": {Data: []byte("SELECT 2;")},
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 1 || migs[0].version != 1 {
		t.Fatalf("只应识别根目录下的 NNNN_name.sql: %+v", migs)
	}
}

func TestLoadMigrationsRejectsMalformedNumberedName(t *testing.T) {
	for _, name := range []string{"2_add.sql", "0002_Add.sql", "0002-add.sql", "0002_.sql", "00002_add.sql"} {
		fsys := fstest.MapFS{name: {Data: []byte("SELECT 1;")}}
		if _, err := loadMigrations(fsys); err == nil {
			t.Errorf("%q 以数字开头但格式不对，应报错而不是被悄悄忽略", name)
		}
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte("SELECT 1;")},
		"0001_b.sql": {Data: []byte("SELECT 2;")},
	}
	if _, err := loadMigrations(fsys); err == nil {
		t.Fatal("同一版本号出现两次应报错")
	}
}

func TestLoadMigrationsRejectsEmptyFile(t *testing.T) {
	fsys := fstest.MapFS{"0001_a.sql": {Data: []byte("  \n-- only comment is fine?\n")}}
	if _, err := loadMigrations(fsys); err != nil {
		t.Fatalf("只有注释的文件不应被当成空文件拒绝: %v", err)
	}
	empty := fstest.MapFS{"0001_a.sql": {Data: []byte(" \n\t\n")}}
	if _, err := loadMigrations(empty); err == nil {
		t.Fatal("全空白文件应报错")
	}
}

func TestChecksumIgnoresLineEndingStyle(t *testing.T) {
	lf := checksum([]byte("CREATE TABLE a (\n  id int\n);\n"))
	crlf := checksum([]byte("CREATE TABLE a (\r\n  id int\r\n);\r\n"))
	if lf != crlf {
		t.Fatal("CRLF 与 LF 应得到同一校验和（Windows 检出的 git autocrlf 不应触发误报）")
	}
	if lf == checksum([]byte("CREATE TABLE b (\n  id int\n);\n")) {
		t.Fatal("内容不同应得到不同校验和")
	}
}

func TestValidateSchemaName(t *testing.T) {
	for _, ok := range []string{"usr", "cpn", "ord_2", "_x"} {
		if err := validateSchema(ok); err != nil {
			t.Errorf("%q 应合法: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Usr", "a-b", "a b", "1abc", `a"b`, "a;drop", "pg_catalog; x", "public.x"} {
		if err := validateSchema(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

func TestAdvisoryKeyStablePerSchema(t *testing.T) {
	if advisoryKey("usr") != advisoryKey("usr") {
		t.Fatal("同一 schema 的锁键必须稳定")
	}
	if advisoryKey("usr") == advisoryKey("cpn") {
		t.Fatal("不同 schema 应使用不同的锁键，互不阻塞")
	}
}
