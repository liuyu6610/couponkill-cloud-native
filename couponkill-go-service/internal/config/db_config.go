package config

import (
	"fmt"
	"log"
	"strings"

	"couponkill-go-service/pkg/sharding"
)

// MySQLConfig MySQL配置
type MySQLConfig struct {
	// 单数据源配置（用于向后兼容）
	DSN      string `yaml:"dsn"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	// 多数据源配置
	DataSources map[string]DataSourceConfig `yaml:"dataSources"`
	// 主从复制配置
	Replication struct {
		Enabled bool               `yaml:"enabled"`
		Master  DataSourceConfig   `yaml:"master"`
		Slaves  []DataSourceConfig `yaml:"slaves"`
	} `yaml:"replication"`
}

// initializeMySQLStandalone 初始化MySQL单节点连接
func initializeMySQLStandalone(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	// 添加默认数据源
	if err := multiDS.AddDataSource("default", cfg.Mysql.DSN); err != nil {
		return nil, fmt.Errorf("添加默认数据源失败: %v", err)
	}

	return multiDS, nil
}

// initializeMySQLCluster 初始化MySQL集群连接
func initializeMySQLCluster(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	// 为每个集群节点创建数据源
	for i, node := range cfg.Middleware.Mysql.Cluster.Nodes {
		// 解析节点信息 host:port
		parts := strings.Split(node, ":")
		if len(parts) != 2 {
			continue
		}

		host := parts[0]
		port := parts[1]

		// 构建DSN
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Mysql.DataSources[fmt.Sprintf("mysql-db-%d", i)].Username,
			cfg.Mysql.DataSources[fmt.Sprintf("mysql-db-%d", i)].Password,
			host, port, cfg.Mysql.DataSources[fmt.Sprintf("mysql-db-%d", i)].Database)

		// 添加数据源
		if err := multiDS.AddDataSource(fmt.Sprintf("mysql-cluster-node-%d", i), dsn); err != nil {
			log.Printf("添加集群节点%d失败: %v", i, err)
			continue
		}
	}

	return multiDS, nil
}

// initializeMySQLReplication 初始化MySQL主从复制连接
func initializeMySQLReplication(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	// 添加主节点
	masterDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Mysql.Replication.Master.Username,
		cfg.Mysql.Replication.Master.Password,
		cfg.Middleware.Mysql.Replication.Master.Host,
		cfg.Middleware.Mysql.Replication.Master.Port,
		cfg.Mysql.Replication.Master.Database)

	if err := multiDS.AddDataSource("mysql-master", masterDSN); err != nil {
		log.Printf("添加主节点失败: %v", err)
	}

	// 添加从节点
	for i, slave := range cfg.Middleware.Mysql.Replication.Slaves {
		slaveDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Mysql.Replication.Slaves[i].Username,
			cfg.Mysql.Replication.Slaves[i].Password,
			slave.Host,
			slave.Port,
			cfg.Mysql.Replication.Slaves[i].Database)

		if err := multiDS.AddDataSource(fmt.Sprintf("mysql-slave-%d", i), slaveDSN); err != nil {
			log.Printf("添加从节点%d失败: %v", i, err)
			continue
		}
	}

	return multiDS, nil
}
