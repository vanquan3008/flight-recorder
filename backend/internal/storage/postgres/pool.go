package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/config"
)

func parsePoolConfig(cfg config.DatabaseConfig) (*pgxpool.Config, error) {
	pool, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection settings")
	}
	pool.MaxConns, pool.MinConns = cfg.MaxConnections, cfg.MinConnections
	pool.MaxConnLifetime = cfg.MaxConnectionLifetime
	pool.MaxConnIdleTime = cfg.MaxConnectionIdleTime
	pool.HealthCheckPeriod = cfg.HealthCheckPeriod
	if pool.ConnConfig.ConnectTimeout == 0 {
		pool.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
		if pool.ConnConfig.ConnectTimeout <= 0 {
			pool.ConnConfig.ConnectTimeout = config.DefaultDatabaseConnectTimeout
		}
	}
	// Spread connection renewals so replicas do not exhaust the pool together.
	if pool.MaxConnLifetimeJitter == 0 {
		pool.MaxConnLifetimeJitter = cfg.MaxConnectionLifetime / 10
	}
	applySessionParameters(pool.ConnConfig, cfg)
	return pool, nil
}
