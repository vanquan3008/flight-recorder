package config

import "fmt"

// LoadMigrations binds and validates only the databases used by the selected command.
func LoadMigrations(database string) (Config, error) {
	if database != "postgres" && database != "clickhouse" && database != "all" {
		return Config{}, fmt.Errorf("unsupported migration database %q", database)
	}
	scopes := []string{"production", "app.production"}
	if database == "postgres" || database == "all" {
		scopes = append(scopes, "database")
	}
	if database == "clickhouse" || database == "all" {
		scopes = append(scopes, "clickhouse", "retention.max_age")
	}
	return loadConfiguration(func(cfg Config) error {
		var v validator
		if database == "postgres" || database == "all" {
			cfg.Database.validateConnectionPool(&v)
			cfg.Database.validateConnectionLifecycle(&v)
			cfg.Database.validateSessionLimits(&v)
			validateDatabaseURL(cfg.Database.URL, cfg.Production, &v)
		}
		if database == "clickhouse" || database == "all" {
			cfg.ClickHouse.validate(&v, cfg.Production)
			if cfg.Retention.MaxAge < 0 {
				v.add("retention.max_age", "must not be negative")
			}
		}
		return v.err()
	}, scopes...)
}
