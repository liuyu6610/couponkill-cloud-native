package config

import "testing"

func TestMergeDBAliasPostgresWins(t *testing.T) {
	c := &Config{}
	c.Postgres.DSN = "host=pg"
	c.Mysql.DSN = "host=legacy"
	mergeDBAlias(c)
	if c.Mysql.DSN != "host=pg" {
		t.Fatalf("expected mysql mirrored from postgres, got %q", c.Mysql.DSN)
	}
}

func TestMergeDBAliasMysqlFallback(t *testing.T) {
	c := &Config{}
	c.Mysql.DataSources = map[string]DataSourceConfig{
		"order-db-0": {DSN: "host=legacy"},
	}
	mergeDBAlias(c)
	if c.Postgres.DataSources["order-db-0"].DSN != "host=legacy" {
		t.Fatalf("expected postgres filled from mysql alias")
	}
}

func TestMergeMiddlewarePostgresWins(t *testing.T) {
	c := &Config{}
	c.Middleware.Postgres.Cluster.Enabled = true
	c.Middleware.Postgres.Cluster.Nodes = []string{"pg:5432"}
	c.Middleware.Mysql.Cluster.Nodes = []string{"legacy:3306"}
	mergeDBAlias(c)
	if len(c.Middleware.Mysql.Cluster.Nodes) != 1 || c.Middleware.Mysql.Cluster.Nodes[0] != "pg:5432" {
		t.Fatalf("expected middleware.mysql mirrored from postgres")
	}
}
