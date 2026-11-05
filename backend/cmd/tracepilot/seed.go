package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
)

const (
	seedTimeout = 20 * time.Second
	adminEmail  = "admin@tracepilot.local"
)

const seedSQLTemplate = `
BEGIN;

INSERT INTO projects (id, slug, name)
VALUES ('{{project_id}}', '{{project_slug}}', '{{project_name}}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO environments (id, project_id, name)
VALUES ('{{environment_id}}', '{{project_id}}', '{{environment_name}}')
ON CONFLICT (project_id, id) DO NOTHING;

INSERT INTO users (id, email, display_name, password_hash, platform_admin)
VALUES (
    '{{user_id}}',
    '{{admin_email}}',
    'Admin',
    'pbkdf2-sha256$600000$FebVOQydajic2r6kpocCfw$6+9j0Sm9rMjtFp90oTSPgqmWRwRuYmiIF9u6bI/cCzs',
    true
)
ON CONFLICT (email) DO NOTHING;

INSERT INTO project_members (project_id, user_id, role)
SELECT '{{project_id}}', id, 'owner'
FROM users
WHERE email = '{{admin_email}}'
ON CONFLICT (project_id, user_id) DO NOTHING;

COMMIT;
`

// newSeedUUID generates a random UUID v4.
func newSeedUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("generate UUID: " + err.Error())
	}

	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b)

	return s[:8] + "-" + s[8:12] + "-" +
		s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

func escapeSQLValue(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func renderSeedSQL(cfg config.IngestionConfig) (string, error) {
	userID, err := newSeedUUID()
	if err != nil {
		return "", err
	}

	replacements := []string{
		"{{project_id}}", escapeSQLValue(cfg.ProjectID),
		"{{environment_id}}", escapeSQLValue(cfg.EnvironmentID),
		"{{project_slug}}", escapeSQLValue(cfg.ProjectSlug),
		"{{project_name}}", escapeSQLValue(cfg.ProjectName),
		"{{environment_name}}", escapeSQLValue(cfg.EnvironmentName),
		"{{admin_email}}", escapeSQLValue(adminEmail),
		"{{user_id}}", userID,
	}

	sql := strings.NewReplacer(replacements...).Replace(seedSQLTemplate)
	if strings.Contains(sql, "{{") || strings.Contains(sql, "}}") {
		return "", errors.New("seed SQL contains unresolved placeholders")
	}
	return sql, nil
}

func runSeed(args []string) int {
	logger := slog.Default()
	if len(args) != 0 {
		logger.Error("invalid seed arguments", "usage", "tracepilot seed")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		return 2
	}
	if cfg.Production {
		logger.Error("seed command is disabled in production")
		return 1
	}

	seedSQL, err := renderSeedSQL(cfg.Ingestion)
	if err != nil {
		logger.Error("failed to prepare seed SQL", "error", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	conn, err := pgx.Connect(ctx, cfg.Database.URL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		return 1
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if err := conn.Close(closeCtx); err != nil {
			logger.Warn("failed to close database connection", "error", err)
		}
	}()

	if _, err := conn.Exec(ctx, seedSQL, pgx.QueryExecModeSimpleProtocol); err != nil {
		logger.Error("seed execution failed", "error", err)
		return 1
	}

	logger.Info("development admin account seeded", "email", adminEmail)
	return 0
}
