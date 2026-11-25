package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/application/replay/capture"
	"github.com/vanquan3008/tracepilot/internal/auth"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type replayRequest struct {
	Target  string            `json:"target"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func (handler *Handler) listExecutions(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	writeChildPage(handler, responseWriter, request, principal, "list allowed request executions",
		func(ctx context.Context, incidentID string, page api.Page) ([]replaydomain.Execution, error) {
			return handler.backend.ListReplayExecutions(ctx, principal.ProjectID, principal.EnvironmentID, incidentID, replaydomain.ExecutionFilter{
				Before: page.Before, CursorID: page.CursorID, Limit: page.Limit,
			})
		},
		func(execution replaydomain.Execution) render.PageCursor {
			return render.PageCursor{At: execution.CreatedAt, ID: execution.ID}
		})
}

func (handler *Handler) getExecution(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	record, ok := handler.load(responseWriter, request, principal)
	if !ok {
		return
	}
	executionID := request.PathValue("executionID")
	if !render.RequireUUID(responseWriter, executionID) {
		return
	}
	execution, queryError := handler.backend.GetReplayExecution(request.Context(), principal.ProjectID, principal.EnvironmentID, record.ID, executionID)
	if queryError != nil {
		handler.errors.Internal(responseWriter, request, "get allowed request execution", queryError)
		return
	}
	if execution.ID == "" {
		render.WriteNotFound(responseWriter, "allowed request execution")
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, execution)
}

// replayRequest sanitizes the captured request and runs it against the incident's target.
func (handler *Handler) replayRequest(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !render.RequireUUID(responseWriter, incidentID) {
		return
	}
	var payload replayRequest
	if decodeError := render.DecodeJSONBody(request, &payload); decodeError != nil {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a target")
		return
	}
	headers := make(http.Header, len(payload.Headers))
	for name, value := range payload.Headers {
		headers.Set(name, value)
	}
	captured, captureError := capture.Sanitize(payload.Method, payload.Target, headers, payload.Body, handler.maxReplayBytes)
	if captureError != nil {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", captureError.Error())
		return
	}
	outcome, runError := handler.replay.Run(request.Context(), replay.Command{
		ProjectID: principal.ProjectID, EnvironmentID: principal.EnvironmentID, IncidentID: incidentID,
		Actor: principal.Actor(), RequestID: request.Header.Get(render.RequestIDHeader), Request: captured,
	})
	if errors.Is(runError, replay.ErrIncidentNotFound) {
		render.WriteNotFound(responseWriter, "incident")
		return
	}
	if runError != nil {
		handler.errors.Internal(responseWriter, request, "run replay", runError)
		return
	}
	render.WriteJSON(responseWriter, http.StatusCreated, outcome)
}
