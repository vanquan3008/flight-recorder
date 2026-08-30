package management

import (
	"errors"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/postgres"
)

const apiKeyHeader = "X-API-Key"

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
		responseWriter.Header().Set("WWW-Authenticate", "Bearer")
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return postgres.Principal{}, false
	}
	if authError != nil {
		server.logger.Error("authenticate query request", "error", authError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return postgres.Principal{}, false
	}
	if !principal.HasScope(scope) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "required API key scope is missing")
		return postgres.Principal{}, false
	}
	return principal, true
}
