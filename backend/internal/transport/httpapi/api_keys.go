package httpapi

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type apiKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (server *Server) listAPIKeys(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	keys, queryError := server.backend.ListAPIKeys(request.Context(), principal.ProjectID, auth.APIKeyFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.logger.Error("list API keys", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(keys, limit, func(key auth.APIKey) pageCursor {
		return pageCursor{At: key.CreatedAt, ID: key.ID}
	}))
}

func (server *Server) createAPIKey(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	var payload apiKeyRequest
	if decodeError := decodeAPIKeyRequest(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	issued, createError := server.backend.CreateAPIKey(request.Context(), principal.ProjectID, principal.KeyID, auth.CreateAPIKeyParams{
		Name: payload.Name, Scopes: payload.Scopes, ExpiresAt: payload.ExpiresAt,
	})
	if writeAPIKeyError(responseWriter, createError) {
		return
	}
	writeIssuedAPIKey(responseWriter, http.StatusCreated, issued)
}

func (server *Server) listAPIKeyEvents(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	limit, parseError := parsePageSize(request.URL.Query().Get("limit"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	cursor, parseError := decodeCursor(request.URL.Query().Get("cursor"))
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	if parseError := validateCursorID(cursor, validUUID); parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", parseError.Error())
		return
	}
	events, queryError := server.backend.ListAPIKeyEvents(request.Context(), principal.ProjectID, auth.APIKeyEventFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.logger.Error("list API key events", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(events, limit, func(event auth.APIKeyEvent) pageCursor {
		return pageCursor{At: event.CreatedAt, ID: event.ID}
	}))
}

func (server *Server) rotateAPIKey(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("keyID")) {
		return
	}
	issued, rotateError := server.backend.RotateAPIKey(request.Context(), principal.ProjectID, request.PathValue("keyID"), principal.KeyID)
	if writeAPIKeyError(responseWriter, rotateError) {
		return
	}
	writeIssuedAPIKey(responseWriter, http.StatusOK, issued)
}

func (server *Server) revokeAPIKey(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	if !requireUUID(responseWriter, request.PathValue("keyID")) {
		return
	}
	_, revokeError := server.backend.RevokeAPIKey(request.Context(), principal.ProjectID, request.PathValue("keyID"), principal.KeyID)
	if writeAPIKeyError(responseWriter, revokeError) {
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	responseWriter.WriteHeader(http.StatusNoContent)
}

func decodeAPIKeyRequest(request *http.Request, payload *apiKeyRequest) error {
	if decodeError := decodeJSONBody(request, payload); decodeError != nil {
		if errors.Is(decodeError, io.EOF) {
			return errors.New("request body must contain name and scopes")
		}
		return decodeError
	}
	return nil
}

func writeAPIKeyError(responseWriter http.ResponseWriter, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, auth.ErrInvalidAPIKeyInput):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
	case errors.Is(operationError, auth.ErrAPIKeyNameExists):
		writeAPIError(responseWriter, http.StatusConflict, "KEY_NAME_EXISTS", "an API key with this name already exists")
	case errors.Is(operationError, auth.ErrAPIKeyNotFound):
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "API key not found")
	case errors.Is(operationError, auth.ErrLastAdminAPIKey):
		writeAPIError(responseWriter, http.StatusConflict, "LAST_ADMIN_KEY", "cannot revoke the last admin API key")
	default:
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

func writeIssuedAPIKey(responseWriter http.ResponseWriter, statusCode int, issued auth.IssuedAPIKey) {
	writeJSONValue(responseWriter, statusCode, map[string]any{"api_key": issued.APIKey, "key": issued.Key})
}
