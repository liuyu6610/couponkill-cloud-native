package postgresclient

import (
	"couponkill-go-service/pkg/sharding"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

// NewMultiClient 初始化多 PostgreSQL 连接。
// 连接池参数由 sharding.AddDataSource 统一设置（可用 GO_DB_MAX_OPEN_CONNS / GO_DB_MAX_IDLE_CONNS 覆盖）。
func NewMultiClient(
	dsn string,
	dataSources map[string]struct {
		DSN string
	},
) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	if len(dataSources) > 0 {
		for name, dsConfig := range dataSources {
			if err := multiDS.AddDataSource(name, dsConfig.DSN); err != nil {
				return nil, fmt.Errorf("failed to add data source %s: %v", name, err)
			}
		}
		return multiDS, nil
	}

	if os.Getenv("GO_DB_MAX_OPEN_CONNS") == "" {
		_ = os.Setenv("GO_DB_MAX_OPEN_CONNS", "100")
	}
	if os.Getenv("GO_DB_MAX_IDLE_CONNS") == "" {
		_ = os.Setenv("GO_DB_MAX_IDLE_CONNS", "20")
	}
	if err := multiDS.AddDataSource("default", dsn); err != nil {
		return nil, fmt.Errorf("PostgreSQL连接失败: %v", err)
	}
	return multiDS, nil
}

// NewMultiClientWithConfig 初始化多 PostgreSQL 连接，使用配置结构体
func NewMultiClientWithConfig(
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

	if len(dataSources) > 0 {
		for name, dsConfig := range dataSources {
			if err := multiDS.AddDataSource(name, dsConfig.DSN); err != nil {
				return nil, fmt.Errorf("failed to add data source %s: %v", name, err)
			}
		}
		return multiDS, nil
	}

	if os.Getenv("GO_DB_MAX_OPEN_CONNS") == "" {
		_ = os.Setenv("GO_DB_MAX_OPEN_CONNS", "100")
	}
	if os.Getenv("GO_DB_MAX_IDLE_CONNS") == "" {
		_ = os.Setenv("GO_DB_MAX_IDLE_CONNS", "20")
	}
	if err := multiDS.AddDataSource("default", dsn); err != nil {
		return nil, fmt.Errorf("PostgreSQL连接失败: %v", err)
	}
	return multiDS, nil
}

// 历史名兼容（一版后可删）
var (
	NewMultiMysqlClient           = NewMultiClient
	NewMultiMysqlClientWithConfig = NewMultiClientWithConfig
)
