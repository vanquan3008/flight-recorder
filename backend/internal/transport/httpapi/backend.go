package httpapi

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/replay"
)

// Backend is the HTTP adapter's port. PostgreSQL implements it, but transport
// code only knows the application/domain contract below.
type Backend interface {
	AuthenticateAPIKey(context.Context, string) (auth.Principal, error)

	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string) ([]query.SpanRecord, error)
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLog(context.Context, string, string, string) (query.LogRecord, error)
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error)

	ListIncidents(context.Context, string, incident.Filter) ([]incident.Incident, error)
	GetIncident(context.Context, string, string, string) (incident.Incident, error)
	ListIncidentOccurrences(context.Context, string, string, string, int, time.Time, string) ([]incident.Occurrence, error)
	ListIncidentEvents(context.Context, string, string, string, int, time.Time, string) ([]incident.Event, error)
	ListIncidentStatusChanges(context.Context, string, string, string, int, time.Time, string) ([]incident.StatusChange, error)
	UpdateIncidentStatus(context.Context, string, string, string, string, string) (incident.Incident, error)

	ListReplayExecutions(context.Context, string, string, string, replaydomain.ExecutionFilter) ([]replaydomain.Execution, error)
	GetReplayExecution(context.Context, string, string, string, string) (replaydomain.Execution, error)

	ListAPIKeys(context.Context, string, auth.APIKeyFilter) ([]auth.APIKey, error)
	CreateAPIKey(context.Context, string, string, auth.CreateAPIKeyParams) (auth.IssuedAPIKey, error)
	ListAPIKeyEvents(context.Context, string, auth.APIKeyEventFilter) ([]auth.APIKeyEvent, error)
	RotateAPIKey(context.Context, string, string, string) (auth.IssuedAPIKey, error)
	RevokeAPIKey(context.Context, string, string, string) (bool, error)
}

type ReplayRunner interface {
	Run(context.Context, replay.Command) (replay.Outcome, error)
}
