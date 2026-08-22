package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flight-observability/flight/internal/capture"
	"github.com/flight-observability/flight/internal/postgres"
	"github.com/flight-observability/flight/internal/replay"
	"github.com/flight-observability/flight/internal/telemetry"
)

const (
	apiKeyHeader      = "X-API-Key"
	defaultPageSize   = 50
	maxPageSize       = 100
	maxLogSearchRange = 24 * time.Hour
)

type pageCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (server *Server) listTraces(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseTraceFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	traces, queryError := server.database.ListTraces(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list traces", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(traces, filter.Limit, func(trace postgres.TraceSummary) pageCursor {
		return pageCursor{At: trace.CursorAt, ID: trace.TraceID}
	}))
}

func (server *Server) getTrace(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	spans, queryError := server.database.GetTrace(request.Context(), principal.ProjectID, request.PathValue("traceID"))
	if queryError != nil {
		server.logger.Error("get trace", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if len(spans) == 0 {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "trace not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"trace_id": request.PathValue("traceID"), "spans": spans})
}

func (server *Server) listTraceLogs(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.TraceID = request.PathValue("traceID")
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	logs, queryError := server.database.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list trace logs", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(logs, filter.Limit, func(record postgres.LogRecord) pageCursor {
		return pageCursor{At: record.OccurredAt, ID: record.EventID}
	}))
}

func (server *Server) listLogs(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseLogFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.Before, filter.CursorID = cursor.At, cursor.ID
	logs, queryError := server.database.ListLogs(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list logs", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(logs, filter.Limit, func(record postgres.LogRecord) pageCursor {
		return pageCursor{At: record.OccurredAt, ID: record.EventID}
	}))
}

func (server *Server) getLog(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	record, queryError := server.database.GetLog(request.Context(), principal.ProjectID, request.PathValue("eventID"))
	if queryError != nil {
		server.logger.Error("get log", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if record.EventID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "log not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, record)
}

func (server *Server) listServices(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	from, fromError := parseTime(request.URL.Query().Get("from"))
	to, toError := parseTime(request.URL.Query().Get("to"))
	if fromError != nil || toError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", "time filters must use RFC3339")
		return
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", rangeError.Error())
		return
	}
	services, queryError := server.database.ListServices(request.Context(), principal.ProjectID, from, to)
	if queryError != nil {
		server.logger.Error("list services", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": services})
}

func (server *Server) listServiceDependencies(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	dependencies, queryError := server.database.ListServiceDependencies(request.Context(), principal.ProjectID, request.PathValue("service"))
	if queryError != nil {
		server.logger.Error("list service dependencies", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"service": request.PathValue("service"), "dependencies": dependencies})
}

func (server *Server) listIncidents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	incidents, queryError := server.database.ListIncidents(request.Context(), principal.ProjectID, limit)
	if queryError != nil {
		server.logger.Error("list incidents", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": incidents})
}

func (server *Server) getIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	incident, queryError := server.database.GetIncident(request.Context(), principal.ProjectID, request.PathValue("incidentID"))
	if queryError != nil {
		server.logger.Error("get incident", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if incident.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, incident)
}

func (server *Server) replayIncident(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "replay")
	if !ok {
		return
	}
	var payload struct {
		Target  string            `json:"target"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a target")
		return
	}
	headers := make(http.Header, len(payload.Headers))
	for name, value := range payload.Headers {
		headers.Set(name, value)
	}
	captured, captureError := capture.Sanitize(payload.Method, payload.Target, headers, payload.Body, server.replayMaxRequestBytes)
	if captureError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", captureError.Error())
		return
	}
	executionID := telemetry.DeterministicID(time.Now(), principal.KeyID, request.PathValue("incidentID"), request.Header.Get(requestIDHeader))
	started, startError := server.database.StartReplayExecution(request.Context(), executionID, principal.ProjectID, request.PathValue("incidentID"), replay.AuditTarget(payload.Target), principal.KeyID)
	if startError != nil {
		server.logger.Error("start replay", "error", startError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if !started {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "incident not found")
		return
	}
	result := server.replay.Execute(request.Context(), captured)
	if completeError := server.database.CompleteReplayExecution(request.Context(), executionID, principal.ProjectID, result); completeError != nil {
		server.logger.Error("complete replay", "error", completeError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"id": executionID, "result": result})
}

func (server *Server) authenticateRead(responseWriter http.ResponseWriter, request *http.Request) (postgres.Principal, bool) {
	return server.authenticateScope(responseWriter, request, "read")
}

func (server *Server) authenticateScope(responseWriter http.ResponseWriter, request *http.Request, scope string) (postgres.Principal, bool) {
	rawKey := strings.TrimSpace(request.Header.Get(apiKeyHeader))
	if rawKey == "" {
		if authorization := request.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			rawKey = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		}
	}
	principal, authError := server.database.AuthenticateAPIKey(request.Context(), rawKey)
	if errors.Is(authError, postgres.ErrUnauthorized) {
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return postgres.Principal{}, false
	}
	if authError != nil {
		server.logger.Error("authenticate query request", "error", authError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return postgres.Principal{}, false
	}
	if !hasScope(principal.Scopes, scope) {
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return postgres.Principal{}, false
	}
	return principal, true
}

func hasScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func parseTraceFilter(request *http.Request) (postgres.TraceFilter, pageCursor, error) {
	query := request.URL.Query()
	limit, parseError := parsePageSize(query.Get("limit"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(query.Get("from"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(query.Get("to"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return postgres.TraceFilter{}, pageCursor{}, rangeError
	}
	cursor, parseError := decodeCursor(query.Get("cursor"))
	if parseError != nil {
		return postgres.TraceFilter{}, pageCursor{}, parseError
	}
	return postgres.TraceFilter{Service: query.Get("service"), Operation: query.Get("operation"), Status: query.Get("status"), TraceID: query.Get("traceId"), From: from, To: to, Limit: limit}, cursor, nil
}

func parseLogFilter(request *http.Request) (postgres.LogFilter, pageCursor, error) {
	query := request.URL.Query()
	limit, parseError := parsePageSize(query.Get("limit"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	from, parseError := parseTime(query.Get("from"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	to, parseError := parseTime(query.Get("to"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	if rangeError := validateTimeRange(from, to); rangeError != nil {
		return postgres.LogFilter{}, pageCursor{}, rangeError
	}
	search := strings.TrimSpace(query.Get("search"))
	if search != "" && (from.IsZero() || to.IsZero()) {
		return postgres.LogFilter{}, pageCursor{}, errors.New("log content search requires from and to")
	}
	if search != "" && to.Sub(from) > maxLogSearchRange {
		return postgres.LogFilter{}, pageCursor{}, errors.New("log content search range must not exceed 24 hours")
	}
	cursor, parseError := decodeCursor(query.Get("cursor"))
	if parseError != nil {
		return postgres.LogFilter{}, pageCursor{}, parseError
	}
	return postgres.LogFilter{Service: query.Get("service"), Severity: query.Get("severity"), TraceID: query.Get("traceId"), SpanID: query.Get("spanId"), Search: search, From: from, To: to, Limit: limit}, cursor, nil
}

func validateTimeRange(from, to time.Time) error {
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return errors.New("from must not be after to")
	}
	return nil
}

func parsePageSize(value string) (int, error) {
	if value == "" {
		return defaultPageSize, nil
	}
	limit, parseError := strconv.Atoi(value)
	if parseError != nil || limit < 1 || limit > maxPageSize {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, parseError := time.Parse(time.RFC3339Nano, value)
	if parseError != nil {
		return time.Time{}, errors.New("time filters must use RFC3339")
	}
	return parsed, nil
}

func decodeCursor(value string) (pageCursor, error) {
	if value == "" {
		return pageCursor{}, nil
	}
	decoded, decodeError := base64.RawURLEncoding.DecodeString(value)
	if decodeError != nil {
		return pageCursor{}, errors.New("cursor is invalid")
	}
	var cursor pageCursor
	if jsonError := json.Unmarshal(decoded, &cursor); jsonError != nil || cursor.At.IsZero() || cursor.ID == "" {
		return pageCursor{}, errors.New("cursor is invalid")
	}
	return cursor, nil
}

func encodeCursor(cursor pageCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func pageResponse[T any](items []T, limit int, cursor func(T) pageCursor) map[string]any {
	response := map[string]any{"data": items}
	if len(items) == limit {
		response["next_cursor"] = encodeCursor(cursor(items[len(items)-1]))
	}
	return response
}

func writeAPIError(responseWriter http.ResponseWriter, statusCode int, code, message string) {
	writeJSONValue(responseWriter, statusCode, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSONValue(responseWriter http.ResponseWriter, statusCode int, value any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(statusCode)
	_ = json.NewEncoder(responseWriter).Encode(value)
}
