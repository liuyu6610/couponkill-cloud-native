package mysqlclient

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// NewMysqlClient 初始化MySQL连接
func NewMysqlClient(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Sprintf("MySQL连接失败: %v", err))
	}
	// 连接池配置
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(20)
	return db
}
