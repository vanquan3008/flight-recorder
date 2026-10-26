package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const apiKeyHeader = "X-API-Key"

func (server *Server) authenticateScope(responseWriter http.ResponseWriter, request *http.Request, scope string) (auth.Principal, bool) {
	rawKey := strings.TrimSpace(request.Header.Get(apiKeyHeader))
	if rawKey == "" {
		if authorization := request.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			rawKey = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		}
	}
	principal, authError := server.backend.AuthenticateAPIKey(request.Context(), rawKey)
	if errors.Is(authError, auth.ErrUnauthorized) {
		responseWriter.Header().Set("WWW-Authenticate", "Bearer")
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, false
	}
	if authError != nil {
		server.writeInternalError(responseWriter, "authenticate API key", authError)
		return auth.Principal{}, false
	}
	if !principal.HasScope(scope) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "required API key scope is missing")
		return auth.Principal{}, false
	}
	return principal, true
}
