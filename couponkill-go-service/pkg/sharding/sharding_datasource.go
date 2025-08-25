package sharding

import (
	"database/sql"
	"fmt"
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

// AddDataSource 添加数据源
func (m *MultiDataSource) AddDataSource(name, dsn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

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
