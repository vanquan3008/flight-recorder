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

func (server *Server) listAPIKeys(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	keys, queryError := server.backend.ListAPIKeys(request.Context(), principal.ProjectID, auth.APIKeyFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.writeInternalError(responseWriter, "list API keys", queryError)
		return
	}
	writePage(responseWriter, keys, limit, func(key auth.APIKey) pageCursor {
		return pageCursor{At: key.CreatedAt, ID: key.ID}
	})
}

func (server *Server) createAPIKey(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload apiKeyRequest
	if decodeError := decodeAPIKeyRequest(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	issued, createError := server.backend.CreateAPIKey(request.Context(), principal.ProjectID, principal.KeyID, auth.CreateAPIKeyParams{
		Name: payload.Name, Scopes: payload.Scopes, ExpiresAt: payload.ExpiresAt,
	})
	if server.writeAPIKeyError(responseWriter, "create API key", createError) {
		return
	}
	writeIssuedAPIKey(responseWriter, http.StatusCreated, issued)
}

func (server *Server) listAPIKeyEvents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	events, queryError := server.backend.ListAPIKeyEvents(request.Context(), principal.ProjectID, auth.APIKeyEventFilter{
		Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.writeInternalError(responseWriter, "list API key events", queryError)
		return
	}
	writePage(responseWriter, events, limit, func(event auth.APIKeyEvent) pageCursor {
		return pageCursor{At: event.CreatedAt, ID: event.ID}
	})
}

func (server *Server) rotateAPIKey(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	keyID := request.PathValue("keyID")
	if !requireUUID(responseWriter, keyID) {
		return
	}
	issued, rotateError := server.backend.RotateAPIKey(request.Context(), principal.ProjectID, keyID, principal.KeyID)
	if server.writeAPIKeyError(responseWriter, "rotate API key", rotateError) {
		return
	}
	writeIssuedAPIKey(responseWriter, http.StatusOK, issued)
}

func (server *Server) revokeAPIKey(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	keyID := request.PathValue("keyID")
	if !requireUUID(responseWriter, keyID) {
		return
	}
	_, revokeError := server.backend.RevokeAPIKey(request.Context(), principal.ProjectID, keyID, principal.KeyID)
	if server.writeAPIKeyError(responseWriter, "revoke API key", revokeError) {
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

// writeAPIKeyError maps API key domain errors to HTTP and reports whether it
// answered the request.
func (server *Server) writeAPIKeyError(responseWriter http.ResponseWriter, operation string, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, auth.ErrInvalidAPIKeyInput):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
	case errors.Is(operationError, auth.ErrAPIKeyNameExists):
		writeAPIError(responseWriter, http.StatusConflict, "KEY_NAME_EXISTS", "an API key with this name already exists")
	case errors.Is(operationError, auth.ErrAPIKeyNotFound):
		writeNotFound(responseWriter, "API key")
	case errors.Is(operationError, auth.ErrLastAdminAPIKey):
		writeAPIError(responseWriter, http.StatusConflict, "LAST_ADMIN_KEY", "cannot revoke the last admin API key")
	default:
		server.writeInternalError(responseWriter, operation, operationError)
	}
	return true
}

func writeIssuedAPIKey(responseWriter http.ResponseWriter, statusCode int, issued auth.IssuedAPIKey) {
	writeJSONValue(responseWriter, statusCode, map[string]any{"api_key": issued.APIKey, "key": issued.Key})
}
