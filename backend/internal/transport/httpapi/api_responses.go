package httpapi

import (
	"github.com/vanquan3008/tracepilot/internal/auth"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// Response envelopes that wrap domain values. Fields are ordered by JSON name so
// the encoding matches the maps these types replaced.

type authStatusResponse struct {
	FirstAccount     bool `json:"first_account"`
	RegistrationOpen bool `json:"registration_open"`
}

type traceResponse struct {
	NextCursor string             `json:"next_cursor,omitempty"`
	Spans      []query.SpanRecord `json:"spans"`
	TraceID    string             `json:"trace_id"`
}

type serviceDependenciesResponse struct {
	Dependencies []query.ServiceDependency `json:"dependencies"`
	Service      string                    `json:"service"`
}

type createProjectResponse struct {
	Environment auth.Environment `json:"environment"`
	Project     auth.Project     `json:"project"`
}

type projectDetailResponse struct {
	Environments []auth.Environment `json:"environments"`
	Project      auth.Project       `json:"project"`
	Role         string             `json:"role"`
}

type projectResponse struct {
	Project auth.Project `json:"project"`
}

type environmentResponse struct {
	Environment auth.Environment `json:"environment"`
}

type memberRoleResponse struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
}

type issuedIngestTokenResponse struct {
	JWT   string           `json:"jwt"`
	Token auth.IngestToken `json:"token"`
}

type createUserResponse struct {
	// TemporaryPassword is present only when the server generated the password.
	TemporaryPassword string    `json:"temporary_password,omitempty"`
	User              auth.User `json:"user"`
}

type userResponse struct {
	User auth.User `json:"user"`
}

type temporaryPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password"`
}

type createWebhookResponse struct {
	Secret  string                     `json:"secret"`
	Webhook notificationdomain.Webhook `json:"webhook"`
}

type webhookSecretResponse struct {
	Secret string `json:"secret"`
}
