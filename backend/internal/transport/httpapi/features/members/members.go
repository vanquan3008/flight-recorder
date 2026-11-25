// Package members serves who belongs to a project, with which role, and the project's audit trail.
package members

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type Backend interface {
	GetUserByEmail(context.Context, string) (auth.User, error)
	ListMembers(context.Context, string) ([]auth.Member, error)
	AddMember(ctx context.Context, projectID, userID, role string, actor auth.Actor) error
	ChangeMemberRole(ctx context.Context, projectID, userID, role string, actor auth.Actor, allowed func(current string) bool) error
	RemoveMember(ctx context.Context, projectID, userID string, actor auth.Actor, allowed func(current string) bool) error
	ListAuditEvents(context.Context, string, auth.AuditFilter) ([]auth.AuditEvent, error)
}

// RoleCache is the router's role cache, cleared when a membership changes.
type RoleCache interface {
	ForgetRole(projectID, userID string)
}

type Handler struct {
	backend Backend
	roles   RoleCache
	errors  api.Errors
}

func New(backend Backend, roles RoleCache, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, roles: roles, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/projects/{projectID}/members", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"projects", "members"}, OperationID: "listProjectMembers", Handler: handler.list, Response: openapi.List[auth.Member]()},
		{Path: "/api/v1/projects/{projectID}/members", Method: http.MethodPost, Scope: auth.ScopeAdmin, Tags: []string{"projects", "members"}, OperationID: "addProjectMember", RequestBody: true, Handler: handler.add, Request: openapi.TypeOf[addMemberRequest](), Response: openapi.Object[auth.Member]()},
		{Path: "/api/v1/projects/{projectID}/members/{userID}", Method: http.MethodPatch, Scope: auth.ScopeAdmin, Tags: []string{"projects", "members"}, OperationID: "changeProjectMemberRole", RequestBody: true, SuccessStatus: http.StatusOK, Handler: handler.changeRole, Request: openapi.TypeOf[changeMemberRoleRequest](), Response: openapi.Object[memberRoleResponse]()},
		{Path: "/api/v1/projects/{projectID}/members/{userID}", Method: http.MethodDelete, Scope: auth.ScopeWrite, Tags: []string{"projects", "members"}, OperationID: "removeProjectMember", Handler: handler.remove, Response: openapi.Empty()},
		{Path: "/api/v1/audit-events", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"audit"}, OperationID: "listAuditEvents", Query: []string{"Limit", "Cursor"}, Handler: handler.listAuditEvents, Response: openapi.Page[auth.AuditEvent]()},
	}
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type changeMemberRoleRequest struct {
	Role string `json:"role"`
}

type memberRoleResponse struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	members, listError := handler.backend.ListMembers(request.Context(), principal.ProjectID)
	if handler.errors.Identity(responseWriter, request, "list members", listError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, map[string]any{"data": members})
}

func (handler *Handler) add(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	var payload addMemberRequest
	if !api.DecodeBody(responseWriter, request, &payload) || !allowedRole(responseWriter, principal, payload.Role) {
		return
	}
	user, lookupError := handler.backend.GetUserByEmail(request.Context(), payload.Email)
	if errors.Is(lookupError, auth.ErrUserNotFound) {
		render.WriteAPIError(responseWriter, http.StatusNotFound, "USER_NOT_FOUND", "no account has this email; a platform administrator can create it")
		return
	}
	if handler.errors.Identity(responseWriter, request, "find user", lookupError) {
		return
	}
	if addError := handler.backend.AddMember(request.Context(), principal.ProjectID, user.ID, payload.Role, principal.Actor()); handler.errors.Identity(responseWriter, request, "add member", addError) {
		return
	}
	handler.roles.ForgetRole(principal.ProjectID, user.ID)
	render.WriteJSON(responseWriter, http.StatusCreated, auth.Member{User: user, Role: payload.Role, CreatedAt: time.Now().UTC()})
}

func (handler *Handler) changeRole(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !api.RequireProject(responseWriter, principal) || !render.RequireUUID(responseWriter, userID) {
		return
	}
	var payload changeMemberRoleRequest
	if !api.DecodeBody(responseWriter, request, &payload) || !allowedRole(responseWriter, principal, payload.Role) {
		return
	}
	changeError := handler.backend.ChangeMemberRole(request.Context(), principal.ProjectID, userID, payload.Role, principal.Actor(),
		func(current string) bool { return auth.CanManageRole(principal.Role, current) })
	if handler.errors.Identity(responseWriter, request, "change member role", changeError) {
		return
	}
	handler.roles.ForgetRole(principal.ProjectID, userID)
	render.WriteJSON(responseWriter, http.StatusOK, memberRoleResponse{UserID: userID, Role: payload.Role})
}

func (handler *Handler) remove(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !api.RequireProject(responseWriter, principal) || !render.RequireUUID(responseWriter, userID) {
		return
	}
	// Anyone may leave a project, so the route is open to every member; removing someone else is for owners.
	if userID != principal.UserID && !auth.CanManageRole(principal.Role, auth.RoleUser) {
		render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "only an owner can remove someone else")
		return
	}
	removeError := handler.backend.RemoveMember(request.Context(), principal.ProjectID, userID, principal.Actor(),
		func(current string) bool {
			return userID == principal.UserID || auth.CanManageRole(principal.Role, current)
		})
	if handler.errors.Identity(responseWriter, request, "remove member", removeError) {
		return
	}
	handler.roles.ForgetRole(principal.ProjectID, userID)
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) listAuditEvents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	api.ListPage(responseWriter, request, func(page api.Page) ([]auth.AuditEvent, error) {
		return handler.backend.ListAuditEvents(request.Context(), principal.ProjectID, auth.AuditFilter{Before: page.Before, CursorID: page.CursorID, Limit: page.Limit})
	}, func(event auth.AuditEvent) render.PageCursor {
		return render.PageCursor{At: event.CreatedAt, ID: event.ID}
	}, func(listError error) {
		handler.errors.Identity(responseWriter, request, "list audit events", listError)
	})
}

// allowedRole answers 400 for an unknown role and 403 for a role the caller cannot grant.
func allowedRole(responseWriter http.ResponseWriter, principal auth.Principal, role string) bool {
	if !auth.ValidRole(role) {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "role must be one of owner, user")
		return false
	}
	if !auth.CanManageRole(principal.Role, role) {
		render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role cannot grant this role")
		return false
	}
	return true
}
