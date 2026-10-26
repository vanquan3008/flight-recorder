package httpapi

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/query"
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
	IncidentBackend
	ReplayBackend
	APIKeyBackend
}

// WebhookBackend is optional at the transport boundary so deployments can run
// without outbound notifications while keeping the rest of the API available.
// PostgreSQL implements it once the schema migrations are applied.
type WebhookBackend interface {
	ListWebhooks(context.Context, string, notificationdomain.WebhookFilter) ([]notificationdomain.Webhook, error)
	GetWebhook(context.Context, string, string) (notificationdomain.Webhook, error)
	CreateWebhook(context.Context, string, string, notificationdomain.CreateWebhookParams) (notificationdomain.IssuedWebhook, error)
	UpdateWebhook(context.Context, string, string, notificationdomain.UpdateWebhookParams) (notificationdomain.Webhook, error)
	DeleteWebhook(context.Context, string, string) (bool, error)
	RotateWebhookSecret(context.Context, string, string) (string, error)
	ListWebhookDeliveries(context.Context, string, notificationdomain.DeliveryFilter) ([]notificationdomain.Delivery, error)
}

// AnalysisBackend is optional so lightweight transport fakes and deployments
// can expose the core API without pretending RCA data exists.
type AnalysisBackend interface {
	ListTopology(context.Context, string, analysis.Filter) (analysis.Topology, error)
	ListMetricPoints(context.Context, string, analysis.Filter) ([]analysis.MetricPoint, error)
	TelemetryQuality(context.Context, string, analysis.Filter) (analysis.Quality, error)
	GetIncidentAnalysis(context.Context, string, string, string) (analysis.IncidentAnalysis, error)
}

type PerformanceBackend interface {
	RequestPerformance(context.Context, string, query.PerformanceFilter) (query.RequestPerformance, error)
}

// AuthBackend authenticates inbound API key tokens.
type AuthBackend interface {
	AuthenticateAPIKey(context.Context, string) (auth.Principal, error)
}

// TelemetryBackend covers traces, spans, logs, and service topology.
type TelemetryBackend interface {
	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string, query.SpanPage) ([]query.SpanRecord, bool, error)
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error)
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
