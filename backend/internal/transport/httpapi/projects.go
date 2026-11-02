package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func (server *Server) listProjects(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	user, userError := server.backend.GetUser(request.Context(), principal.UserID)
	if server.writeIdentityError(responseWriter, "get user", userError) {
		return
	}
	memberships, listError := server.projectsFor(request, user)
	if server.writeIdentityError(responseWriter, "list projects", listError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": memberships})
}

func (server *Server) createProject(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Environment string `json:"environment"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	// A regular account may own a handful of projects; a super administrator is not limited.
	ownedLimit := auth.MaxOwnedProjects
	if principal.PlatformAdmin {
		ownedLimit = 0
	}
	project, environment, createError := server.backend.CreateProject(request.Context(), auth.CreateProjectParams{
		Slug: payload.Slug, Name: payload.Name, EnvironmentName: payload.Environment,
	}, principal.UserID, ownedLimit, principal.Actor())
	if server.writeIdentityError(responseWriter, "create project", createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"project": project, "environment": environment})
}

func (server *Server) getProject(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	project, projectError := server.backend.GetProject(request.Context(), principal.ProjectID)
	if server.writeIdentityError(responseWriter, "get project", projectError) {
		return
	}
	environments, listError := server.backend.ListEnvironments(request.Context(), principal.ProjectID)
	if server.writeIdentityError(responseWriter, "list environments", listError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"project": project, "role": principal.Role, "environments": environments})
}

func (server *Server) renameProject(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	project, renameError := server.backend.RenameProject(request.Context(), principal.ProjectID, payload.Name, principal.Actor())
	if server.writeIdentityError(responseWriter, "rename project", renameError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"project": project})
}

func (server *Server) createEnvironment(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	environment, createError := server.backend.CreateEnvironment(request.Context(), principal.ProjectID, payload.Name, principal.Actor())
	if server.writeIdentityError(responseWriter, "create environment", createError) {
		return
	}
	// Readers cache a project's environments briefly; this server sees the new one at once.
	server.environments.forget(principal.ProjectID)
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"environment": environment})
}

func (server *Server) listMembers(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	members, listError := server.backend.ListMembers(request.Context(), principal.ProjectID)
	if server.writeIdentityError(responseWriter, "list members", listError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"data": members})
}

func (server *Server) addMember(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	var payload struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	if !auth.ValidRole(payload.Role) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "role must be one of owner, user")
		return
	}
	if !auth.CanManageRole(principal.Role, payload.Role) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role cannot grant this role")
		return
	}
	user, lookupError := server.backend.GetUserByEmail(request.Context(), payload.Email)
	if errors.Is(lookupError, auth.ErrUserNotFound) {
		writeAPIError(responseWriter, http.StatusNotFound, "USER_NOT_FOUND", "no account has this email; a platform administrator can create it")
		return
	}
	if server.writeIdentityError(responseWriter, "find user", lookupError) {
		return
	}
	if addError := server.backend.AddMember(request.Context(), principal.ProjectID, user.ID, payload.Role, principal.Actor()); server.writeIdentityError(responseWriter, "add member", addError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, auth.Member{User: user, Role: payload.Role, CreatedAt: time.Now().UTC()})
}

func (server *Server) changeMemberRole(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !requireProjectAccess(responseWriter, request, principal) || !requireUUID(responseWriter, userID) {
		return
	}
	var payload struct {
		Role string `json:"role"`
	}
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	if !auth.ValidRole(payload.Role) {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "role must be one of owner, user")
		return
	}
	if !auth.CanManageRole(principal.Role, payload.Role) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role cannot grant this role")
		return
	}
	changeError := server.backend.ChangeMemberRole(request.Context(), principal.ProjectID, userID, payload.Role, principal.Actor(),
		func(current string) bool { return auth.CanManageRole(principal.Role, current) })
	if server.writeIdentityError(responseWriter, "change member role", changeError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]any{"user_id": userID, "role": payload.Role})
}

func (server *Server) removeMember(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !requireProjectAccess(responseWriter, request, principal) || !requireUUID(responseWriter, userID) {
		return
	}
	// Anyone may leave a project, so the route is open to every member; removing someone else is for owners.
	if userID != principal.UserID && !auth.CanManageRole(principal.Role, auth.RoleUser) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "only an owner can remove someone else")
		return
	}
	removeError := server.backend.RemoveMember(request.Context(), principal.ProjectID, userID, principal.Actor(),
		func(current string) bool {
			return userID == principal.UserID || auth.CanManageRole(principal.Role, current)
		})
	if server.writeIdentityError(responseWriter, "remove member", removeError) {
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (server *Server) listAuditEvents(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	events, listError := server.backend.ListAuditEvents(request.Context(), principal.ProjectID, auth.AuditFilter{Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if server.writeIdentityError(responseWriter, "list audit events", listError) {
		return
	}
	writePage(responseWriter, events, limit, func(event auth.AuditEvent) pageCursor {
		return pageCursor{At: event.CreatedAt, ID: event.ID}
	})
}
