package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

func (server *Server) analysisBackend(responseWriter http.ResponseWriter) (AnalysisBackend, bool) {
	backend, ok := server.backend.(AnalysisBackend)
	if !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "NOT_IMPLEMENTED", "observability analysis is unavailable")
	}
	return backend, ok
}

func parseAnalysisFilter(request *http.Request) (analysis.Filter, error) {
	values := request.URL.Query()
	from, err := parseTime(values.Get("from"))
	if err != nil {
		return analysis.Filter{}, errors.New("time filters must use RFC3339")
	}
	to, err := parseTime(values.Get("to"))
	if err != nil {
		return analysis.Filter{}, errors.New("time filters must use RFC3339")
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-15 * time.Minute)
	}
	if err := validateTimeRange(from, to); err != nil {
		return analysis.Filter{}, err
	}
	if to.Sub(from) > 30*24*time.Hour {
		return analysis.Filter{}, errors.New("analysis time range must not exceed 30 days")
	}
	limit, err := parsePageSize(values.Get("limit"))
	if err != nil {
		return analysis.Filter{}, err
	}
	return analysis.Filter{EnvironmentID: "", Service: strings.TrimSpace(values.Get("service")), Metric: strings.TrimSpace(values.Get("metric")), From: from, To: to, Limit: limit}, nil
}

// analysisRequest resolves the analysis backend and the request's filter,
// scoped to this server's environment, answering the error itself when either
// is unavailable.
func (server *Server) analysisRequest(responseWriter http.ResponseWriter, request *http.Request) (AnalysisBackend, analysis.Filter, bool) {
	backend, ok := server.analysisBackend(responseWriter)
	if !ok {
		return nil, analysis.Filter{}, false
	}
	filter, err := parseAnalysisFilter(request)
	if err != nil {
		writeInvalidQuery(responseWriter, err)
		return nil, analysis.Filter{}, false
	}
	filter.EnvironmentID = server.environmentID
	return backend, filter, true
}

func (server *Server) listTopology(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request)
	if !ok {
		return
	}
	topology, err := backend.ListTopology(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "list topology", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, topology)
}

func (server *Server) listMetricPoints(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request)
	if !ok {
		return
	}
	points, err := backend.ListMetricPoints(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "list metric points", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": points})
}

func (server *Server) telemetryQuality(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, filter, ok := server.analysisRequest(responseWriter, request)
	if !ok {
		return
	}
	quality, err := backend.TelemetryQuality(request.Context(), principal.ProjectID, filter)
	if err != nil {
		server.writeInternalError(responseWriter, "telemetry quality", err)
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, quality)
}

func (server *Server) getIncidentAnalysis(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	incidentID := request.PathValue("incidentID")
	if !validUUID(incidentID) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_ID", "incident ID must be a UUID")
		return
	}
	backend, ok := server.analysisBackend(responseWriter)
	if !ok {
		return
	}
	result, err := backend.GetIncidentAnalysis(request.Context(), principal.ProjectID, server.environmentID, incidentID)
	if err != nil {
		server.writeInternalError(responseWriter, "get incident analysis", err)
		return
	}
	if result.IncidentID == "" {
		writeNotFound(responseWriter, "incident")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, result)
}
