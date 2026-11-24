package config

import "fmt"

type Config struct {
	App           AppConfig           `config:"app"`
	HTTP          HTTPConfig          `config:"http"`
	Database      DatabaseConfig      `config:"database"`
	ClickHouse    ClickHouseConfig    `config:"clickhouse"`
	Auth          AuthConfig          `config:"auth"`
	Cache         CacheConfig         `config:"cache"`
	Storage       StorageConfig       `config:"storage"`
	Queue         QueueConfig         `config:"queue"`
	Ingestion     IngestionConfig     `config:"ingestion"`
	Observability ObservabilityConfig `config:"observability"`
	Replay        ReplayConfig        `config:"replay"`
	Notifications NotificationsConfig `config:"notifications"`
}

func Load() (Config, error) {
	return loadConfiguration(Config.Validate)
}

func loadConfiguration(validate func(Config) error, scopes ...string) (Config, error) {
	cfg := defaultConfig()

	fileValues, err := readEnvironmentFile()
	if err != nil {
		return Config{}, fmt.Errorf("read environment file: %w", err)
	}

	if err := applyEnvironment(&cfg, fileValues, scopes...); err != nil {
		return Config{}, fmt.Errorf("apply environment: %w", err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate configuration: %w", err)
	}

	return cfg, nil
}
