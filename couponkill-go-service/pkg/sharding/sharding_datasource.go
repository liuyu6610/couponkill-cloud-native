package sharding

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// MultiDataSource 多数据源管理器
type MultiDataSource struct {
	dataSources map[string]*sql.DB
	mu          sync.RWMutex
}

// NewMultiDataSource 创建多数据源管理器
func NewMultiDataSource() *MultiDataSource {
	return &MultiDataSource{
		dataSources: make(map[string]*sql.DB),
	}
}

func poolIntEnv(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

// AddDataSource 添加数据源。
// 默认 MaxOpenConns=64（可通过 GO_DB_MAX_OPEN_CONNS 覆盖），避免旧默认 25 在多 DS 下成为隐藏并发天花板。
func (m *MultiDataSource) AddDataSource(name, dsn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	maxOpen := poolIntEnv("GO_DB_MAX_OPEN_CONNS", 64)
	maxIdle := poolIntEnv("GO_DB_MAX_IDLE_CONNS", 16)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	m.dataSources[name] = db
	return nil
}

// GetDataSource 获取数据源
func (m *MultiDataSource) GetDataSource(name string) (*sql.DB, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	db, exists := m.dataSources[name]
	if !exists {
		return nil, fmt.Errorf("data source %s not found", name)
	}

	return db, nil
}

// Close 关闭所有数据源
func (m *MultiDataSource) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, db := range m.dataSources {
		if err := db.Close(); err != nil {
			return err
		}
	}

	return nil
}
