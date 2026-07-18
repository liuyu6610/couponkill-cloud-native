package config

import (
	"fmt"
	"log"
	"strings"

	"couponkill-go-service/pkg/sharding"
)

// initializePostgresStandalone 初始化 PostgreSQL 单节点连接
func initializePostgresStandalone(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()

	dsn := cfg.Postgres.DSN
	if dsn == "" {
		dsn = cfg.Mysql.DSN
	}
	if err := multiDS.AddDataSource("default", dsn); err != nil {
		return nil, fmt.Errorf("添加默认数据源失败: %v", err)
	}

	return multiDS, nil
}

// initializePostgresCluster 初始化 PG 集群连接（读 middleware.postgres；兼容旧 middleware.mysql）
func initializePostgresCluster(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()
	mw := cfg.Middleware.Postgres
	if mw.empty() {
		mw = cfg.Middleware.Mysql
	}

	for i, node := range mw.Cluster.Nodes {
		parts := strings.Split(node, ":")
		if len(parts) != 2 {
			continue
		}

		host := parts[0]
		port := parts[1]

		dsKey := fmt.Sprintf("postgres-db-%d", i)
		legacyKey := fmt.Sprintf("mysql-db-%d", i)
		ds := cfg.Postgres.DataSources[dsKey]
		if ds.Username == "" {
			ds = cfg.Postgres.DataSources[legacyKey]
		}
		if ds.Username == "" {
			ds = cfg.Mysql.DataSources[legacyKey]
		}

		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			host, port, ds.Username, ds.Password, ds.Database)

		if err := multiDS.AddDataSource(fmt.Sprintf("pg-cluster-node-%d", i), dsn); err != nil {
			log.Printf("添加集群节点%d失败: %v", i, err)
			continue
		}
	}

	return multiDS, nil
}

// initializePostgresReplication 初始化 PG 主从连接
func initializePostgresReplication(cfg *Config) (*sharding.MultiDataSource, error) {
	multiDS := sharding.NewMultiDataSource()
	mw := cfg.Middleware.Postgres
	if mw.empty() {
		mw = cfg.Middleware.Mysql
	}

	masterCred := cfg.Postgres.Replication.Master
	if masterCred.Username == "" {
		masterCred = cfg.Mysql.Replication.Master
	}

	masterDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		mw.Replication.Master.Host,
		mw.Replication.Master.Port,
		masterCred.Username,
		masterCred.Password,
		masterCred.Database)

	if err := multiDS.AddDataSource("pg-master", masterDSN); err != nil {
		log.Printf("添加主节点失败: %v", err)
	}

	for i, slave := range mw.Replication.Slaves {
		slaveCred := DataSourceConfig{}
		if i < len(cfg.Postgres.Replication.Slaves) {
			slaveCred = cfg.Postgres.Replication.Slaves[i]
		} else if i < len(cfg.Mysql.Replication.Slaves) {
			slaveCred = cfg.Mysql.Replication.Slaves[i]
		}
		slaveDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			slave.Host,
			slave.Port,
			slaveCred.Username,
			slaveCred.Password,
			slaveCred.Database)

		if err := multiDS.AddDataSource(fmt.Sprintf("pg-slave-%d", i), slaveDSN); err != nil {
			log.Printf("添加从节点%d失败: %v", i, err)
			continue
		}
	}

	return multiDS, nil
}

// 历史名保留，避免外部引用编译失败
var (
	initializeMySQLStandalone  = initializePostgresStandalone
	initializeMySQLCluster     = initializePostgresCluster
	initializeMySQLReplication = initializePostgresReplication
)
