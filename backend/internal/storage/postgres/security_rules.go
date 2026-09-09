package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func (database *Database) ListSecurityRules(ctx context.Context, projectID string) ([]security.Rule, error) {
	rows, queryError := database.pool.Query(ctx, `SELECT id, name, COALESCE(description, ''), version, enabled, severity, match_field, match_operator, match_value, threshold_count, COALESCE(group_by_field, ''), window_seconds, created_at, updated_at FROM security_detection_rules WHERE project_id = $1 ORDER BY name, id`, projectID)
	if queryError != nil {
		return nil, fmt.Errorf("list security rules: %w", queryError)
	}
	defer rows.Close()
	rules := make([]security.Rule, 0)
	for rows.Next() {
		rule, scanError := scanSecurityRule(rows)
		if scanError != nil {
			return nil, scanError
		}
		rules = append(rules, rule)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate security rules: %w", rowsError)
	}
	return rules, nil
}

func (database *Database) CreateSecurityRule(ctx context.Context, projectID, actorKeyID string, rule security.Rule) (security.Rule, error) {
	if normalizeRule(&rule) != nil {
		return security.Rule{}, security.ErrInvalidRule
	}
	now := time.Now().UTC()
	rule.ID = telemetry.DeterministicID(now, "security-rule", projectID, rule.Name)
	rule.Version = 1
	rule.CreatedAt = now
	rule.UpdatedAt = now
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return security.Rule{}, fmt.Errorf("begin create security rule: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	_, executeError := transaction.Exec(ctx, `INSERT INTO security_detection_rules (id, project_id, name, description, version, enabled, severity, match_field, match_operator, match_value, threshold_count, group_by_field, window_seconds, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, rule.ID, projectID, rule.Name, nullableString(rule.Description), rule.Version, rule.Enabled, rule.Severity, rule.Field, rule.Operator, rule.Value, rule.Threshold, nullableString(rule.GroupBy), int(rule.Window.Seconds()), now, now)
	if executeError != nil {
		return security.Rule{}, fmt.Errorf("create security rule: %w", executeError)
	}
	if auditError := insertSecurityRuleAudit(ctx, transaction, projectID, actorKeyID, rule, "CREATED"); auditError != nil {
		return security.Rule{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return security.Rule{}, fmt.Errorf("commit create security rule: %w", commitError)
	}
	return rule, nil
}

func (database *Database) UpdateSecurityRule(ctx context.Context, projectID, ruleID, actorKeyID string, rule security.Rule) (security.Rule, error) {
	if normalizeRule(&rule) != nil {
		return security.Rule{}, security.ErrInvalidRule
	}
	now := time.Now().UTC()
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return security.Rule{}, fmt.Errorf("begin update security rule: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	row := transaction.QueryRow(ctx, `UPDATE security_detection_rules SET name=$3, description=$4, version=version+1, enabled=$5, severity=$6, match_field=$7, match_operator=$8, match_value=$9, threshold_count=$10, group_by_field=$11, window_seconds=$12, updated_at=$13 WHERE project_id=$1 AND id=$2 RETURNING id, name, COALESCE(description, ''), version, enabled, severity, match_field, match_operator, match_value, threshold_count, COALESCE(group_by_field, ''), window_seconds, created_at, updated_at`, projectID, ruleID, rule.Name, nullableString(rule.Description), rule.Enabled, rule.Severity, rule.Field, rule.Operator, rule.Value, rule.Threshold, nullableString(rule.GroupBy), int(rule.Window.Seconds()), now)
	updated, scanError := scanSecurityRule(row)
	if errors.Is(scanError, pgx.ErrNoRows) {
		return security.Rule{}, nil
	}
	if scanError != nil {
		return security.Rule{}, fmt.Errorf("update security rule: %w", scanError)
	}
	if auditError := insertSecurityRuleAudit(ctx, transaction, projectID, actorKeyID, updated, "UPDATED"); auditError != nil {
		return security.Rule{}, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return security.Rule{}, fmt.Errorf("commit update security rule: %w", commitError)
	}
	return updated, nil
}

func (database *Database) DeleteSecurityRule(ctx context.Context, projectID, ruleID, actorKeyID string) (bool, error) {
	// Detection records reference a rule ID and version. Rules are therefore
	// disabled instead of deleted so historical evidence remains explainable.
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return false, fmt.Errorf("begin disable security rule: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	row := transaction.QueryRow(ctx, "UPDATE security_detection_rules SET enabled=false, version=version+1, updated_at=now() WHERE project_id=$1 AND id=$2 AND enabled=true RETURNING id, name, COALESCE(description, ''), version, enabled, severity, match_field, match_operator, match_value, threshold_count, COALESCE(group_by_field, ''), window_seconds, created_at, updated_at", projectID, ruleID)
	disabled, scanError := scanSecurityRule(row)
	if errors.Is(scanError, pgx.ErrNoRows) {
		return false, nil
	}
	if scanError != nil {
		return false, fmt.Errorf("disable security rule: %w", scanError)
	}
	if auditError := insertSecurityRuleAudit(ctx, transaction, projectID, actorKeyID, disabled, "DISABLED"); auditError != nil {
		return false, auditError
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return false, fmt.Errorf("commit disable security rule: %w", commitError)
	}
	return true, nil
}

func insertSecurityRuleAudit(ctx context.Context, transaction pgx.Tx, projectID, actorKeyID string, rule security.Rule, action string) error {
	snapshot, marshalError := json.Marshal(rule)
	if marshalError != nil {
		return fmt.Errorf("encode security rule audit: %w", marshalError)
	}
	auditID := telemetry.DeterministicID(rule.UpdatedAt, "security-rule-audit", projectID, rule.ID, fmt.Sprint(rule.Version), action)
	if _, executeError := transaction.Exec(ctx, `INSERT INTO security_rule_audits (id, project_id, rule_id, rule_version, action, actor_key_id, rule_snapshot) VALUES ($1,$2,$3,$4,$5,$6,$7)`, auditID, projectID, rule.ID, rule.Version, action, nullableString(actorKeyID), snapshot); executeError != nil {
		return fmt.Errorf("insert security rule audit: %w", executeError)
	}
	return nil
}

func (database *Database) ListSecurityRuleAudits(ctx context.Context, projectID, ruleID string, limit int) ([]security.RuleAudit, error) {
	rows, queryError := database.pool.Query(ctx, `SELECT id, rule_id, rule_version, action, COALESCE(actor_key_id::text, ''), rule_snapshot, created_at FROM security_rule_audits WHERE project_id=$1 AND rule_id=$2 ORDER BY created_at DESC, id DESC LIMIT $3`, projectID, ruleID, limit)
	if queryError != nil {
		return nil, fmt.Errorf("list security rule audits: %w", queryError)
	}
	defer rows.Close()
	audits := make([]security.RuleAudit, 0, limit)
	for rows.Next() {
		var audit security.RuleAudit
		var snapshot []byte
		if scanError := rows.Scan(&audit.ID, &audit.RuleID, &audit.Version, &audit.Action, &audit.ActorKeyID, &snapshot, &audit.CreatedAt); scanError != nil {
			return nil, fmt.Errorf("scan security rule audit: %w", scanError)
		}
		if unmarshalError := json.Unmarshal(snapshot, &audit.Rule); unmarshalError != nil {
			return nil, fmt.Errorf("decode security rule audit: %w", unmarshalError)
		}
		audits = append(audits, audit)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate security rule audits: %w", rowsError)
	}
	return audits, nil
}

func scanSecurityRule(row securityEventScanner) (security.Rule, error) {
	var rule security.Rule
	var seconds int
	if scanError := row.Scan(&rule.ID, &rule.Name, &rule.Description, &rule.Version, &rule.Enabled, &rule.Severity, &rule.Field, &rule.Operator, &rule.Value, &rule.Threshold, &rule.GroupBy, &seconds, &rule.CreatedAt, &rule.UpdatedAt); scanError != nil {
		return security.Rule{}, scanError
	}
	rule.Window = time.Duration(seconds) * time.Second
	rule.WindowSeconds = seconds
	return rule, nil
}

func normalizeRule(rule *security.Rule) error {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Description = strings.TrimSpace(rule.Description)
	rule.Severity = strings.ToUpper(strings.TrimSpace(rule.Severity))
	rule.Field = strings.ToLower(strings.TrimSpace(rule.Field))
	rule.Operator = strings.ToLower(strings.TrimSpace(rule.Operator))
	rule.Value = strings.TrimSpace(rule.Value)
	rule.GroupBy = strings.ToLower(strings.TrimSpace(rule.GroupBy))
	if rule.Severity == "" {
		rule.Severity = "MEDIUM"
	}
	if rule.Window == 0 && rule.WindowSeconds > 0 {
		rule.Window = time.Duration(rule.WindowSeconds) * time.Second
	}
	rule.WindowSeconds = int(rule.Window.Seconds())
	return rule.Validate()
}
