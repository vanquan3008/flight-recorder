package config

import "fmt"

func LoadMigrations(database string) (Config, error) {
	includePostgres := database == "postgres" || database == "all"
	includeClickHouse := database == "clickhouse" || database == "all"
	if !includePostgres && !includeClickHouse {
		return Config{}, fmt.Errorf("unsupported migration database %q", database)
	}
	scopes := []string{"production", "app.production"}
	if includePostgres {
		scopes = append(scopes, "database")
	}
	if includeClickHouse {
		scopes = append(scopes, "clickhouse")
	}
	return loadConfiguration(func(cfg Config) error {
		var v validator
		if includePostgres {
			cfg.Database.validateConnectionPool(&v)
			cfg.Database.validateConnectionLifecycle(&v)
			cfg.Database.validateSessionLimits(&v)
			cfg.Database.validateMigrations(&v)
			validateDatabaseURL(cfg.Database.URL, cfg.Production, &v)
		}
		if includeClickHouse {
			cfg.ClickHouse.validate(&v, cfg.Production)
		}
		return v.err()
	}, scopes...)
}
