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
		scopes = append(scopes, "clickhouse", "retention.max_age")
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
			if cfg.Retention.MaxAge < 0 {
				v.add("retention.max_age", "must not be negative")
			}
		}
		return v.err()
	}, scopes...)
}
