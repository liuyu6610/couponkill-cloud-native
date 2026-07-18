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
