package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

var projectRoutes = []apiRouteDefinition{
	{path: "/api/v1/projects", method: http.MethodGet, scope: auth.ScopeSession, tags: []string{"projects"}, operationID: "listProjects", handler: (*Server).listProjects, response: openapi.List[auth.Membership]()},
	{path: "/api/v1/projects", method: http.MethodPost, scope: auth.ScopeSession, tags: []string{"projects"}, operationID: "createProject", requestBody: true, handler: (*Server).createProject, request: openapi.TypeOf[createProjectRequest](), response: openapi.Object[createProjectResponse]()},
	{path: "/api/v1/projects/{projectID}", method: http.MethodGet, scope: "read", tags: []string{"projects"}, operationID: "getProject", handler: (*Server).getProject, response: openapi.Object[projectDetailResponse]()},
	{path: "/api/v1/projects/{projectID}", method: http.MethodPatch, scope: "admin", tags: []string{"projects"}, operationID: "updateProject", requestBody: true, successStatus: http.StatusOK, handler: (*Server).renameProject, request: openapi.TypeOf[renameProjectRequest](), response: openapi.Object[projectResponse]()},
	{path: "/api/v1/projects/{projectID}/retention", method: http.MethodGet, scope: "read", tags: []string{"projects"}, operationID: "getProjectRetention", handler: (*Server).getProjectRetention, response: openapi.Object[projectRetentionResponse]()},
	{path: "/api/v1/projects/{projectID}/retention", method: http.MethodPut, scope: "admin", tags: []string{"projects"}, operationID: "updateProjectRetention", requestBody: true, successStatus: http.StatusOK, handler: (*Server).updateProjectRetention, request: openapi.TypeOf[projectRetentionRequest](), response: openapi.Object[projectRetentionResponse]()},
	{path: "/api/v1/projects/{projectID}/environments", method: http.MethodPost, scope: "admin", tags: []string{"projects"}, operationID: "createEnvironment", requestBody: true, handler: (*Server).createEnvironment, request: openapi.TypeOf[createEnvironmentRequest](), response: openapi.Object[environmentResponse]()},
	{path: "/api/v1/projects/{projectID}/members", method: http.MethodGet, scope: "admin", tags: []string{"projects", "members"}, operationID: "listProjectMembers", handler: (*Server).listMembers, response: openapi.List[auth.Member]()},
	{path: "/api/v1/projects/{projectID}/members", method: http.MethodPost, scope: "admin", tags: []string{"projects", "members"}, operationID: "addProjectMember", requestBody: true, handler: (*Server).addMember, request: openapi.TypeOf[addMemberRequest](), response: openapi.Object[auth.Member]()},
	{path: "/api/v1/projects/{projectID}/members/{userID}", method: http.MethodPatch, scope: "admin", tags: []string{"projects", "members"}, operationID: "changeProjectMemberRole", requestBody: true, successStatus: http.StatusOK, handler: (*Server).changeMemberRole, request: openapi.TypeOf[changeMemberRoleRequest](), response: openapi.Object[memberRoleResponse]()},
	{path: "/api/v1/projects/{projectID}/members/{userID}", method: http.MethodDelete, scope: "write", tags: []string{"projects", "members"}, operationID: "removeProjectMember", handler: (*Server).removeMember, response: openapi.Empty()},
	{path: "/api/v1/projects/{projectID}/ingest-tokens", method: http.MethodGet, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "listIngestTokens", query: []string{"Limit", "Cursor"}, handler: (*Server).listIngestTokens, response: openapi.Page[auth.IngestToken]()},
	{path: "/api/v1/projects/{projectID}/ingest-tokens", method: http.MethodPost, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "createIngestToken", requestBody: true, handler: (*Server).createIngestToken, request: openapi.TypeOf[createIngestTokenRequest](), response: openapi.Object[issuedIngestTokenResponse]()},
	{path: "/api/v1/projects/{projectID}/ingest-tokens/{tokenID}", method: http.MethodDelete, scope: "admin", tags: []string{"projects", "ingest-tokens"}, operationID: "revokeIngestToken", handler: (*Server).revokeIngestToken, response: openapi.Empty()},
	{path: "/api/v1/audit-events", method: http.MethodGet, scope: "admin", tags: []string{"audit"}, operationID: "listAuditEvents", query: []string{"Limit", "Cursor"}, handler: (*Server).listAuditEvents, response: openapi.Page[auth.AuditEvent]()},
}

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
	var payload createProjectRequest
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
	writeJSONValue(responseWriter, http.StatusCreated, createProjectResponse{Project: project, Environment: environment})
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
	writeJSONValue(responseWriter, http.StatusOK, projectDetailResponse{Project: project, Role: principal.Role, Environments: environments})
}

func (server *Server) renameProject(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	var payload renameProjectRequest
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	project, renameError := server.backend.RenameProject(request.Context(), principal.ProjectID, payload.Name, principal.Actor())
	if server.writeIdentityError(responseWriter, "rename project", renameError) {
		return
	}
	if server.identity != nil {
		server.identity.forgetProject(principal.ProjectID, "")
	}
	writeJSONValue(responseWriter, http.StatusOK, projectResponse{Project: project})
}

func (server *Server) createEnvironment(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	var payload createEnvironmentRequest
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
	writeJSONValue(responseWriter, http.StatusCreated, environmentResponse{Environment: environment})
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
	var payload addMemberRequest
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
	if server.identity != nil {
		server.identity.forgetRole(principal.ProjectID, user.ID)
	}
	writeJSONValue(responseWriter, http.StatusCreated, auth.Member{User: user, Role: payload.Role, CreatedAt: time.Now().UTC()})
}

func (server *Server) changeMemberRole(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	userID := request.PathValue("userID")
	if !requireProjectAccess(responseWriter, request, principal) || !requireUUID(responseWriter, userID) {
		return
	}
	var payload changeMemberRoleRequest
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
	if server.identity != nil {
		server.identity.forgetRole(principal.ProjectID, userID)
	}
	writeJSONValue(responseWriter, http.StatusOK, memberRoleResponse{UserID: userID, Role: payload.Role})
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
	if server.identity != nil {
		server.identity.forgetRole(principal.ProjectID, userID)
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

// createIngestToken issues the credential a telemetry sender uses for this project's OTLP endpoint. The
// JWT is returned once; the owner can list and revoke the token afterwards.
func (server *Server) createIngestToken(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload createIngestTokenRequest
	if decodeError := decodeIdentityBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	params := auth.CreateIngestTokenParams{Name: payload.Name}
	if payload.ExpiresAt != nil {
		params.ExpiresAt = *payload.ExpiresAt
	}
	issued, createError := server.backend.CreateIngestToken(request.Context(), principal.ProjectID, principal.Actor(), params,
		func(tokenID string, expiresAt time.Time) (string, error) {
			return auth.IssueIngestToken(server.ingestJWTSecret, principal.ProjectID, tokenID, time.Now(), expiresAt)
		})
	if server.writeIdentityError(responseWriter, "create ingest token", createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, issuedIngestTokenResponse{Token: issued.Token, JWT: issued.JWT})
}

func (server *Server) listIngestTokens(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !requireProjectAccess(responseWriter, request, principal) {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	tokens, listError := server.backend.ListIngestTokens(request.Context(), principal.ProjectID, auth.IngestTokenFilter{Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if server.writeIdentityError(responseWriter, "list ingest tokens", listError) {
		return
	}
	writePage(responseWriter, tokens, limit, func(token auth.IngestToken) pageCursor {
		return pageCursor{At: token.CreatedAt, ID: token.ID}
	})
}

func (server *Server) revokeIngestToken(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	tokenID := request.PathValue("tokenID")
	if !requireProjectAccess(responseWriter, request, principal) || !requireUUID(responseWriter, tokenID) {
		return
	}
	if revokeError := server.backend.RevokeIngestToken(request.Context(), principal.ProjectID, tokenID, principal.Actor()); server.writeIdentityError(responseWriter, "revoke ingest token", revokeError) {
		return
	}
	if server.tokenInvalidator != nil {
		server.tokenInvalidator.Invalidate(tokenID)
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}
