package mysqlclient

import (
	"couponkill-go-service/pkg/sharding"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// NewMultiMysqlClient 初始化多MySQL连接
func NewMultiMysqlClient(
	dsn string,
	dataSources map[string]struct {
		DSN string
	},
) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	// 如果配置了多数据源
	if len(dataSources) > 0 {
		for name, dsConfig := range dataSources {
			if err := multiDS.AddDataSource(name, dsConfig.DSN); err != nil {
				return nil, fmt.Errorf("failed to add data source %s: %v", name, err)
			}
		}
	} else {
		// 向后兼容，如果只配置了单个DSN，则添加默认数据源
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("MySQL连接失败: %v", err)
		}
		// 连接池配置
		db.SetMaxOpenConns(100)
		db.SetMaxIdleConns(20)

		// 添加默认数据源
		multiDS.AddDataSource("default", dsn)
	}

	return multiDS, nil
}

// NewMultiMysqlClientWithConfig 初始化多MySQL连接，使用配置结构体
func NewMultiMysqlClientWithConfig(
	dsn string,
	dataSources map[string]struct {
		DSN      string
		Username string
		Password string
		Host     string
		Port     int
		Database string
	},
) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	// 如果配置了多数据源
	if len(dataSources) > 0 {
		for name, dsConfig := range dataSources {
			if err := multiDS.AddDataSource(name, dsConfig.DSN); err != nil {
				return nil, fmt.Errorf("failed to add data source %s: %v", name, err)
			}
		}
	} else {
		// 向后兼容，如果只配置了单个DSN，则添加默认数据源
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("MySQL连接失败: %v", err)
		}
		// 连接池配置
		db.SetMaxOpenConns(100)
		db.SetMaxIdleConns(20)

		// 添加默认数据源
		multiDS.AddDataSource("default", dsn)
	}

	return multiDS, nil
}
