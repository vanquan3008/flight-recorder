package httpapi

import (
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

var userRoutes = []apiRouteDefinition{
	{path: "/api/v1/users", method: http.MethodGet, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "listUsers", handler: (*Server).listUsers},
	{path: "/api/v1/users", method: http.MethodPost, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "createUser", requestBody: true, handler: (*Server).createUser},
	{path: "/api/v1/users/{userID}", method: http.MethodPatch, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "updateUser", requestBody: true, successStatus: http.StatusOK, handler: (*Server).updateUser},
	{path: "/api/v1/users/{userID}/password-reset", method: http.MethodPost, scope: auth.ScopePlatform, tags: []string{"users"}, operationID: "resetUserPassword", successStatus: http.StatusOK, handler: (*Server).resetUserPassword},
}

func (server *Server) listUsers(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	users, listError := server.backend.ListUsers(request.Context(), auth.UserFilter{Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if server.writeIdentityError(responseWriter, "list users", listError) {
		return
	}
	writePage(responseWriter, users, limit, func(user auth.User) pageCursor {
		return pageCursor{At: user.CreatedAt, ID: user.ID}
	})
}

// createUser makes an account with a password the administrator hands over once; the person must
// replace it at first sign-in. Without a supplied password one is generated.
func (server *Server) createUser(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload struct {
		Email         string `json:"email"`
		DisplayName   string `json:"display_name"`
		Password      string `json:"password"`
		PlatformAdmin bool   `json:"platform_admin"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	password, generated := payload.Password, false
	if password == "" {
		var passwordError error
		if password, passwordError = auth.NewTemporaryPassword(); passwordError != nil {
			server.writeInternalError(responseWriter, "generate password", passwordError)
			return
		}
		generated = true
	}
	user, createError := server.backend.CreateUser(request.Context(), auth.CreateUserParams{
		Email: payload.Email, DisplayName: payload.DisplayName, Password: password,
		PlatformAdmin: payload.PlatformAdmin, MustChangePassword: true,
	}, principal.Actor())
	if server.writeIdentityError(responseWriter, "create user", createError) {
		return
	}
	response := map[string]any{"user": user}
	if generated {
		response["temporary_password"] = password
	}
	writeJSONValue(responseWriter, http.StatusCreated, response)
}

func (server *Server) updateUser(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !requireUUID(responseWriter, userID) {
		return
	}
	var payload struct {
		DisplayName   *string `json:"display_name"`
		PlatformAdmin *bool   `json:"platform_admin"`
		Disabled      *bool   `json:"disabled"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	user, updateError := server.backend.UpdateUser(request.Context(), userID, auth.UpdateUserParams{
		DisplayName: payload.DisplayName, PlatformAdmin: payload.PlatformAdmin, Disabled: payload.Disabled,
	}, principal.Actor())
	if server.writeIdentityError(responseWriter, "update user", updateError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"user": user})
}

// resetUserPassword issues a new temporary password and ends the account's sessions.
func (server *Server) resetUserPassword(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !requireUUID(responseWriter, userID) {
		return
	}
	password, passwordError := auth.NewTemporaryPassword()
	if passwordError != nil {
		server.writeInternalError(responseWriter, "generate password", passwordError)
		return
	}
	if setError := server.backend.SetPassword(request.Context(), userID, password, true, "", principal.Actor()); server.writeIdentityError(responseWriter, "reset password", setError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"temporary_password": password})
}
