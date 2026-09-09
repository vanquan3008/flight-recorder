package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

func (server *Server) listSecurityEvents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	filter, cursor, parseError := parseSecurityFilter(request)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	filter.EnvironmentID, filter.Before, filter.CursorID = server.environmentID, cursor.At, cursor.ID
	events, queryError := server.backend.ListSecurityEvents(request.Context(), principal.ProjectID, filter)
	if queryError != nil {
		server.logger.Error("list security events", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(events, filter.Limit, func(event security.Event) pageCursor { return pageCursor{At: event.OccurredAt, ID: event.ID} }))
}

func (server *Server) getSecurityEvent(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	eventID := request.PathValue("eventID")
	if !requireUUID(responseWriter, eventID) {
		return
	}
	event, queryError := server.backend.GetSecurityEvent(request.Context(), principal.ProjectID, server.environmentID, eventID)
	if queryError != nil {
		server.logger.Error("get security event", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if event.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "security event not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, event)
}

func (server *Server) listSecuritySources(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	sources, queryError := server.backend.ListSecuritySources(request.Context(), principal.ProjectID, server.environmentID, limit)
	if queryError != nil {
		server.logger.Error("list security sources", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": sources})
}

func (server *Server) listSecurityRules(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	rules, queryError := server.backend.ListSecurityRules(request.Context(), principal.ProjectID)
	if queryError != nil {
		server.logger.Error("list security rules", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": rules})
}

func (server *Server) createSecurityRule(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "write")
	if !ok {
		return
	}
	rule, decodeError := decodeSecurityRule(request)
	if decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	created, createError := server.backend.CreateSecurityRule(request.Context(), principal.ProjectID, principal.KeyID, rule)
	if writeSecurityRuleError(responseWriter, createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, created)
}

func (server *Server) updateSecurityRule(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "write")
	if !ok {
		return
	}
	ruleID := request.PathValue("ruleID")
	if !requireUUID(responseWriter, ruleID) {
		return
	}
	rule, decodeError := decodeSecurityRule(request)
	if decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	updated, updateError := server.backend.UpdateSecurityRule(request.Context(), principal.ProjectID, ruleID, principal.KeyID, rule)
	if writeSecurityRuleError(responseWriter, updateError) {
		return
	}
	if updated.ID == "" {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "security rule not found")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, updated)
}

func (server *Server) deleteSecurityRule(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "write")
	if !ok {
		return
	}
	ruleID := request.PathValue("ruleID")
	if !requireUUID(responseWriter, ruleID) {
		return
	}
	deleted, deleteError := server.backend.DeleteSecurityRule(request.Context(), principal.ProjectID, ruleID, principal.KeyID)
	if deleteError != nil {
		server.logger.Error("delete security rule", "error", deleteError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if !deleted {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "security rule not found")
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (server *Server) listSecurityRuleAudits(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	ruleID := request.PathValue("ruleID")
	if !requireUUID(responseWriter, ruleID) {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	audits, queryError := server.backend.ListSecurityRuleAudits(request.Context(), principal.ProjectID, ruleID, limit)
	if queryError != nil {
		server.logger.Error("list security rule audits", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": audits})
}

func (server *Server) listSecurityDetections(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateRead(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	detections, queryError := server.backend.ListSecurityDetections(request.Context(), principal.ProjectID, server.environmentID, limit, cursor.At, cursor.ID)
	if queryError != nil {
		server.logger.Error("list security detections", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(detections, limit, func(detection security.Detection) pageCursor {
		return pageCursor{At: detection.CreatedAt, ID: detection.ID}
	}))
}

func decodeSecurityRule(request *http.Request) (security.Rule, error) {
	var rule security.Rule
	if decodeError := decodeJSONBody(request, &rule); decodeError != nil {
		return security.Rule{}, decodeError
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Window = time.Duration(rule.WindowSeconds) * time.Second
	if validationError := rule.Validate(); validationError != nil {
		return security.Rule{}, validationError
	}
	return rule, nil
}

func writeSecurityRuleError(responseWriter http.ResponseWriter, operationError error) bool {
	if operationError == nil {
		return false
	}
	if errors.Is(operationError, security.ErrInvalidRule) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
		return true
	}
	writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	return true
}
