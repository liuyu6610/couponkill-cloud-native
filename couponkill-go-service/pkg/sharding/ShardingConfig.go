package sharding

// ShardingConfig represents the ShardingSphere configuration structure
type ShardingConfig struct {
	DataSources map[string]DataSourceConfig `yaml:"dataSources"`
	Rules       []ShardingRule              `yaml:"rules"`
	Props       map[string]interface{}      `yaml:"props"`
}

// DataSourceConfig represents a data source configuration
type DataSourceConfig struct {
	DriverClassName     string `yaml:"driverClassName"`
	JDBCURL             string `yaml:"jdbcUrl"`
	Username            string `yaml:"username"`
	Password            string `yaml:"password"`
	DataSourceClassName string `yaml:"dataSourceClassName"`
}

// ShardingRule represents a sharding rule configuration
type ShardingRule struct {
	Tables             map[string]TableRule `yaml:"tables"`
	ShardingAlgorithms map[string]Algorithm `yaml:"shardingAlgorithms"`
	KeyGenerators      map[string]Generator `yaml:"keyGenerators"`
}

// TableRule represents a table rule configuration
type TableRule struct {
	ActualDataNodes  string         `yaml:"actualDataNodes"`
	DatabaseStrategy StrategyConfig `yaml:"databaseStrategy"`
	TableStrategy    StrategyConfig `yaml:"tableStrategy"`
}

// StrategyConfig represents a strategy configuration
type StrategyConfig struct {
	Standard StandardStrategy `yaml:"standard"`
}

// StandardStrategy represents a standard strategy configuration
type StandardStrategy struct {
	ShardingColumn        string `yaml:"shardingColumn"`
	ShardingAlgorithmName string `yaml:"shardingAlgorithmName"`
}

// Algorithm represents a sharding algorithm configuration
type Algorithm struct {
	Type  string            `yaml:"type"`
	Props map[string]string `yaml:"props"`
}

// Generator represents a key generator configuration
type Generator struct {
	Type  string            `yaml:"type"`
	Props map[string]string `yaml:"props"`
}
