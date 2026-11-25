// Package users serves the accounts the super administrator manages.
package users

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type Backend interface {
	CreateUser(context.Context, auth.CreateUserParams, auth.Actor) (auth.User, error)
	ListUsers(context.Context, auth.UserFilter) ([]auth.User, error)
	UpdateUser(ctx context.Context, userID string, update auth.UpdateUserParams, actor auth.Actor) (auth.User, error)
	SetPassword(ctx context.Context, userID, newPassword string, mustChange bool, keepToken string, actor auth.Actor) error
}

type Handler struct {
	backend Backend
	errors  api.Errors
}

func New(backend Backend, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/users", Method: http.MethodGet, Scope: auth.ScopePlatform, Tags: []string{"users"}, OperationID: "listUsers", Query: []string{"Limit", "Cursor"}, Handler: handler.list, Response: openapi.Page[auth.User]()},
		{Path: "/api/v1/users", Method: http.MethodPost, Scope: auth.ScopePlatform, Tags: []string{"users"}, OperationID: "createUser", RequestBody: true, Handler: handler.create, Request: openapi.TypeOf[createUserRequest](), Response: openapi.Object[createUserResponse]()},
		{Path: "/api/v1/users/{userID}", Method: http.MethodPatch, Scope: auth.ScopePlatform, Tags: []string{"users"}, OperationID: "updateUser", RequestBody: true, SuccessStatus: http.StatusOK, Handler: handler.update, Request: openapi.TypeOf[updateUserRequest](), Response: openapi.Object[userResponse]()},
		{Path: "/api/v1/users/{userID}/password-reset", Method: http.MethodPost, Scope: auth.ScopePlatform, Tags: []string{"users"}, OperationID: "resetUserPassword", SuccessStatus: http.StatusOK, Handler: handler.resetPassword, Response: openapi.Object[temporaryPasswordResponse]()},
	}
}

type createUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type updateUserRequest struct {
	DisplayName *string `json:"display_name"`
	Disabled    *bool   `json:"disabled"`
}

type createUserResponse struct {
	// TemporaryPassword is present only when the server generated the password.
	TemporaryPassword string    `json:"temporary_password,omitempty"`
	User              auth.User `json:"user"`
}

type userResponse struct {
	User auth.User `json:"user"`
}

type temporaryPasswordResponse struct {
	TemporaryPassword string `json:"temporary_password"`
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, _ auth.Principal) {
	api.ListPage(responseWriter, request, func(page api.Page) ([]auth.User, error) {
		return handler.backend.ListUsers(request.Context(), auth.UserFilter{Before: page.Before, CursorID: page.CursorID, Limit: page.Limit})
	}, func(user auth.User) render.PageCursor {
		return render.PageCursor{At: user.CreatedAt, ID: user.ID}
	}, func(listError error) { handler.errors.Identity(responseWriter, request, "list users", listError) })
}

// create makes an account with a password the administrator hands over once; the person must
// replace it at first sign-in. Without a supplied password one is generated.
func (handler *Handler) create(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload createUserRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	password, generated := payload.Password, false
	if password == "" {
		var passwordError error
		if password, passwordError = auth.NewTemporaryPassword(); passwordError != nil {
			handler.errors.Internal(responseWriter, request, "generate password", passwordError)
			return
		}
		generated = true
	}
	user, createError := handler.backend.CreateUser(request.Context(), auth.CreateUserParams{
		Email: payload.Email, DisplayName: payload.DisplayName, Password: password, MustChangePassword: true,
	}, principal.Actor())
	if handler.errors.Identity(responseWriter, request, "create user", createError) {
		return
	}
	response := createUserResponse{User: user}
	if generated {
		response.TemporaryPassword = password
	}
	render.WriteJSON(responseWriter, http.StatusCreated, response)
}

func (handler *Handler) update(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !render.RequireUUID(responseWriter, userID) {
		return
	}
	var payload updateUserRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	user, updateError := handler.backend.UpdateUser(request.Context(), userID, auth.UpdateUserParams{
		DisplayName: payload.DisplayName, Disabled: payload.Disabled,
	}, principal.Actor())
	if handler.errors.Identity(responseWriter, request, "update user", updateError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, userResponse{User: user})
}

// resetPassword issues a new temporary password and ends the account's sessions.
func (handler *Handler) resetPassword(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !render.RequireUUID(responseWriter, userID) {
		return
	}
	password, passwordError := auth.NewTemporaryPassword()
	if passwordError != nil {
		handler.errors.Internal(responseWriter, request, "generate password", passwordError)
		return
	}
	if setError := handler.backend.SetPassword(request.Context(), userID, password, true, "", principal.Actor()); handler.errors.Identity(responseWriter, request, "reset password", setError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, temporaryPasswordResponse{TemporaryPassword: password})
}
