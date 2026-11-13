// Package identity stores accounts, sessions, projects and membership, and
// the audit trail of changes to them.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

const (
	// A refresh session ends after sessionIdleTTL without a refresh, and never lasts past sessionMaxTTL.
	sessionIdleTTL = 7 * 24 * time.Hour
	sessionMaxTTL  = 30 * 24 * time.Hour

	// Advisory lock keys serialize checks that span several rows.
	setupLockKey      int64 = 7_203_411_004
	membershipLockKey int64 = 7_203_411_005
	adminLockKey      int64 = 7_203_411_006
)

type IdentityStore struct {
	pool    *pgxpool.Pool
	queries *sqlcstore.Queries
}

func New(pool *pgxpool.Pool) *IdentityStore {
	return &IdentityStore{pool: pool, queries: sqlcstore.New(pool)}
}

func newID() string {
	var entropy [16]byte
	if _, readError := rand.Read(entropy[:]); readError != nil {
		panic(fmt.Sprintf("read random bytes: %v", readError))
	}
	return telemetry.DeterministicID(time.Now().UTC(), string(entropy[:]))
}

func newToken() (string, error) {
	return auth.RandomToken(32)
}

func hashToken(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

func userFrom(row sqlcstore.User) auth.User {
	return auth.User{
		ID: row.ID, Email: row.Email, DisplayName: row.DisplayName, PlatformAdmin: row.PlatformAdmin,
		MustChangePassword: row.MustChangePassword, DisabledAt: row.DisabledAt, CreatedAt: row.CreatedAt, LastLoginAt: row.LastLoginAt,
		TokensValidAfter: row.TokensValidAfter,
	}
}

func projectFrom(row sqlcstore.Project) auth.Project {
	return auth.Project{ID: row.ID, Slug: row.Slug, Name: row.Name, CreatedAt: row.CreatedAt}
}

func environmentFrom(row sqlcstore.Environment) auth.Environment {
	return auth.Environment{ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, CreatedAt: row.CreatedAt}
}

// audit records a control-plane change in the caller's transaction, so the change and its record commit together.
func audit(ctx context.Context, queries *sqlcstore.Queries, projectID string, actor auth.Actor, action, targetType, targetID string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	encoded, marshalError := json.Marshal(detail)
	if marshalError != nil {
		return fmt.Errorf("encode audit detail: %w", marshalError)
	}
	if insertError := queries.InsertAuditEvent(ctx, sqlcstore.InsertAuditEventParams{
		ID: newID(), ProjectID: pgvalue.OrNil(projectID), ActorUserID: pgvalue.OrNil(actor.UserID),
		Action: action, TargetType: targetType, TargetID: targetID, Detail: encoded,
	}); insertError != nil {
		return fmt.Errorf("record audit event: %w", insertError)
	}
	return nil
}

// inTransaction runs work under one transaction with queries bound to it.
func (store *IdentityStore) inTransaction(ctx context.Context, work func(*sqlcstore.Queries) error) error {
	return pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		return work(store.queries.WithTx(transaction))
	})
}

// ListAuditEvents returns a project's control-plane history, newest first.
func (store *IdentityStore) ListAuditEvents(ctx context.Context, projectID string, filter auth.AuditFilter) ([]auth.AuditEvent, error) {
	rows, queryError := store.queries.ListAuditEvents(ctx, sqlcstore.ListAuditEventsParams{
		ProjectID: &projectID, Before: pgvalue.TimeOrNil(filter.Before), CursorID: pgvalue.OrNil(filter.CursorID), RowLimit: int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list audit events: %w", queryError)
	}
	events := make([]auth.AuditEvent, 0, len(rows))
	for _, row := range rows {
		event := auth.AuditEvent{
			ID: row.ID, Action: row.Action, TargetType: row.TargetType, TargetID: row.TargetID, CreatedAt: row.CreatedAt,
			ProjectID: pgvalue.StringOrEmpty(row.ProjectID), ActorUserID: pgvalue.StringOrEmpty(row.ActorUserID),
		}
		if unmarshalError := json.Unmarshal(row.Detail, &event.Detail); unmarshalError != nil {
			return nil, fmt.Errorf("decode audit detail: %w", unmarshalError)
		}
		events = append(events, event)
	}
	return events, nil
}
