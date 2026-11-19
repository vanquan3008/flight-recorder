package httpapi

import (
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

type projectRetentionRequest struct {
	Days *int32 `json:"retention_days"`
}

type projectRetentionResponse struct {
	Days              *int32 `json:"retention_days"`
	DefaultAgeSeconds int64  `json:"default_age_seconds"`
	MaxDays           int64  `json:"max_days"`
}

func (server *Server) writeProjectRetention(responseWriter http.ResponseWriter, days *int32) {
	defaultSeconds := int64(server.defaultRetentionAge / time.Second)
	if server.defaultRetentionAge > 0 {
		defaultSeconds = max(1, defaultSeconds)
	}
	writeJSONValue(responseWriter, http.StatusOK, projectRetentionResponse{Days: days, DefaultAgeSeconds: defaultSeconds, MaxDays: telemetry.MaxRetentionDays})
}

func (server *Server) getProjectRetention(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	days, err := server.backend.GetProjectRetention(request.Context(), principal.ProjectID)
	if !server.writeIdentityError(responseWriter, "read project retention", err) {
		server.writeProjectRetention(responseWriter, days)
	}
}

func (server *Server) updateProjectRetention(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	// An explicit null inherits the default. A missing field leaves the invalid sentinel intact.
	missing := int32(-1)
	payload := projectRetentionRequest{Days: &missing}
	if err := decodeIdentityBody(request, &payload); err != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if days := payload.Days; days != nil && (*days < 1 || int64(*days) > telemetry.MaxRetentionDays) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "retention_days must be a positive supported number of days or null to inherit the default")
		return
	}
	if err := server.backend.UpdateProjectRetention(request.Context(), principal.ProjectID, payload.Days, principal.Actor()); !server.writeIdentityError(responseWriter, "update project retention", err) {
		server.writeProjectRetention(responseWriter, payload.Days)
	}
}
