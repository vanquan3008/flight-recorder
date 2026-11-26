package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

func TestWebhookEventConstraintsIntegration(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	hook, err := database.CreateWebhook(ctx, scope.ProjectID, "", notificationdomain.CreateWebhookParams{
		Name: "event-constraints", URL: "https://example.com/hook", Events: []string{"incident.opened"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, events := range []string{
		`{incident.opened}`, `{incident.resolved}`, `{incident.opened,incident.resolved}`,
	} {
		if _, err := database.pool.Exec(ctx, "UPDATE webhooks SET events = $1::text[] WHERE id = $2", events, hook.ID); err != nil {
			t.Fatalf("valid events %s: %v", events, err)
		}
	}
	for _, events := range []string{`{}`, `{NULL}`, `{incident.opened,NULL}`, `{unknown.event}`, `{{incident.opened,incident.resolved}}`} {
		t.Run(events, func(t *testing.T) {
			_, err := database.pool.Exec(ctx, "UPDATE webhooks SET events = $1::text[] WHERE id = $2", events, hook.ID)
			var violation *pgconn.PgError
			if !errors.As(err, &violation) || violation.Code != "23514" {
				t.Fatalf("invalid events %s: got %v, want a check violation", events, err)
			}
		})
	}
}

func TestIncidentStateConstraintsIntegration(t *testing.T) {
	database, scope := newFlowTestDatabase(t)
	ctx := context.Background()
	var id string
	err := database.pool.QueryRow(ctx, `INSERT INTO incidents (
		id, project_id, environment_id, primary_service, fingerprint, title,
		severity, status, first_seen_at, last_seen_at
	) VALUES (gen_random_uuid(), $1, $2, 'api', 'constraints', 'State constraints', 'ERROR', 'OPEN', now(), now())
	RETURNING id`, scope.ProjectID, scope.EnvironmentID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"OPEN", "INVESTIGATING", "RESOLVED", "IGNORED"} {
		if _, err := database.pool.Exec(ctx, "UPDATE incidents SET status = $1 WHERE id = $2", status, id); err != nil {
			t.Fatalf("valid status %s: %v", status, err)
		}
	}
	for _, severity := range []string{"ERROR", "CRITICAL"} {
		if _, err := database.pool.Exec(ctx, "UPDATE incidents SET severity = $1 WHERE id = $2", severity, id); err != nil {
			t.Fatalf("valid severity %s: %v", severity, err)
		}
	}
	for _, test := range []struct {
		name, statement string
	}{
		{"invalid status", "UPDATE incidents SET status = 'INVALID' WHERE id = $1"},
		{"empty status", "UPDATE incidents SET status = '' WHERE id = $1"},
		{"invalid severity", "UPDATE incidents SET severity = 'unknown' WHERE id = $1"},
		{"empty severity", "UPDATE incidents SET severity = '' WHERE id = $1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := database.pool.Exec(ctx, test.statement, id)
			var violation *pgconn.PgError
			if !errors.As(err, &violation) || violation.Code != "23514" {
				t.Fatalf("got %v, want a check violation", err)
			}
		})
	}
}
