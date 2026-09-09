package httpapi

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/replay"
)

// Backend is the HTTP adapter's port. PostgreSQL implements it, but transport
// code only knows the application/domain contract below.
//
// It is composed of narrow, domain-scoped interfaces so each handler group
// depends only on the methods it actually calls. This makes fakes in tests
// cheaper and keeps the dependency surface explicit.
type Backend interface {
	AuthBackend
	TelemetryBackend
	SecurityBackend
	IncidentBackend
	ReplayBackend
	APIKeyBackend
}

// WebhookBackend is optional at the transport boundary so deployments can run
// without outbound notifications while keeping the rest of the API available.
// PostgreSQL implements it when migration 13 is installed.
type WebhookBackend interface {
	ListWebhooks(context.Context, string, notificationdomain.WebhookFilter) ([]notificationdomain.Webhook, error)
	GetWebhook(context.Context, string, string) (notificationdomain.Webhook, error)
	CreateWebhook(context.Context, string, string, notificationdomain.CreateWebhookParams) (notificationdomain.IssuedWebhook, error)
	UpdateWebhook(context.Context, string, string, notificationdomain.UpdateWebhookParams) (notificationdomain.Webhook, error)
	DeleteWebhook(context.Context, string, string) (bool, error)
	RotateWebhookSecret(context.Context, string, string) (string, error)
	ListWebhookDeliveries(context.Context, string, notificationdomain.DeliveryFilter) ([]notificationdomain.Delivery, error)
}

// AuthBackend authenticates inbound API key tokens.
type AuthBackend interface {
	AuthenticateAPIKey(context.Context, string) (auth.Principal, error)
}

// TelemetryBackend covers traces, spans, logs, and service topology.
type TelemetryBackend interface {
	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string) ([]query.SpanRecord, error)
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error)
}

// SecurityBackend covers security events, sources, detection rules, and detections.
type SecurityBackend interface {
	ListSecurityEvents(context.Context, string, security.Filter) ([]security.Event, error)
	GetSecurityEvent(context.Context, string, string, string) (security.Event, error)
	ListSecuritySources(context.Context, string, string, int) ([]security.Source, error)
	ListSecurityRules(context.Context, string) ([]security.Rule, error)
	CreateSecurityRule(context.Context, string, string, security.Rule) (security.Rule, error)
	UpdateSecurityRule(context.Context, string, string, string, security.Rule) (security.Rule, error)
	DeleteSecurityRule(context.Context, string, string, string) (bool, error)
	ListSecurityRuleAudits(context.Context, string, string, int) ([]security.RuleAudit, error)
	ListSecurityDetections(context.Context, string, string, int, time.Time, string) ([]security.Detection, error)
}

// IncidentBackend covers incident lifecycle and timeline queries.
type IncidentBackend interface {
	ListIncidents(context.Context, string, incident.Filter) ([]incident.Incident, error)
	GetIncident(context.Context, string, string, string) (incident.Incident, error)
	ListIncidentOccurrences(context.Context, string, string, string, int, time.Time, string) ([]incident.Occurrence, error)
	ListIncidentEvents(context.Context, string, string, string, int, time.Time, string) ([]incident.Event, error)
	ListIncidentStatusChanges(context.Context, string, string, string, int, time.Time, string) ([]incident.StatusChange, error)
	UpdateIncidentStatus(context.Context, string, string, string, string, string) (incident.Incident, error)
}

// ReplayBackend stores and retrieves replay execution records.
type ReplayBackend interface {
	ListReplayExecutions(context.Context, string, string, string, replaydomain.ExecutionFilter) ([]replaydomain.Execution, error)
	GetReplayExecution(context.Context, string, string, string, string) (replaydomain.Execution, error)
}

// APIKeyBackend manages API key CRUD and audit events.
type APIKeyBackend interface {
	ListAPIKeys(context.Context, string, auth.APIKeyFilter) ([]auth.APIKey, error)
	CreateAPIKey(context.Context, string, string, auth.CreateAPIKeyParams) (auth.IssuedAPIKey, error)
	ListAPIKeyEvents(context.Context, string, auth.APIKeyEventFilter) ([]auth.APIKeyEvent, error)
	RotateAPIKey(context.Context, string, string, string) (auth.IssuedAPIKey, error)
	RevokeAPIKey(context.Context, string, string, string) (bool, error)
}

// ReplayRunner executes a captured HTTP request against a live target.
type ReplayRunner interface {
	Run(context.Context, replay.Command) (replay.Outcome, error)
}
