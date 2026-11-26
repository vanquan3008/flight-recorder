// Package projects serves projects, their environments, and their retention.
package projects

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type Backend interface {
	api.MembershipLister
	GetUser(context.Context, string) (auth.User, error)
	CreateProject(ctx context.Context, params auth.CreateProjectParams, creatorUserID string, ownedLimit int, actor auth.Actor) (auth.Project, auth.Environment, error)
	GetProject(context.Context, string) (auth.Project, error)
	RenameProject(ctx context.Context, projectID, name string, actor auth.Actor) (auth.Project, error)
	ListEnvironments(context.Context, string) ([]auth.Environment, error)
	CreateEnvironment(ctx context.Context, projectID, name string, actor auth.Actor) (auth.Environment, error)
}

type RetentionBackend interface {
	GetProjectRetention(ctx context.Context, projectID string) (*int32, error)
	UpdateProjectRetention(ctx context.Context, projectID string, days *int32, actor auth.Actor) error
}

// Cache is the router's project and environment cache, cleared when a project changes.
type Cache interface {
	ForgetProject(projectID string)
	ForgetEnvironments(projectID string)
}

type Handler struct {
	backend   Backend
	retention RetentionBackend
	cache     Cache
	errors    api.Errors
	// defaultRetention is the retention a project without its own setting inherits.
	defaultRetention time.Duration
}

func New(backend Backend, retention RetentionBackend, cache Cache, defaultRetention time.Duration, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, retention: retention, cache: cache, errors: api.Errors{Logger: logger}, defaultRetention: defaultRetention}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/projects", Method: http.MethodGet, Scope: auth.ScopeSession, Tags: []string{"projects"}, OperationID: "listProjects", Handler: handler.list, Response: openapi.List[auth.Membership]()},
		{Path: "/api/v1/projects", Method: http.MethodPost, Scope: auth.ScopeSession, Tags: []string{"projects"}, OperationID: "createProject", RequestBody: true, Handler: handler.create, Request: openapi.TypeOf[createProjectRequest](), Response: openapi.Object[createProjectResponse]()},
		{Path: "/api/v1/projects/{projectID}", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"projects"}, OperationID: "getProject", Handler: handler.get, Response: openapi.Object[projectDetailResponse]()},
		{Path: "/api/v1/projects/{projectID}", Method: http.MethodPatch, Scope: auth.ScopeAdmin, Tags: []string{"projects"}, OperationID: "updateProject", RequestBody: true, SuccessStatus: http.StatusOK, Handler: handler.rename, Request: openapi.TypeOf[renameProjectRequest](), Response: openapi.Object[projectResponse]()},
		{Path: "/api/v1/projects/{projectID}/retention", Method: http.MethodGet, Scope: auth.ScopeRead, Tags: []string{"projects"}, OperationID: "getProjectRetention", Handler: handler.getRetention, Response: openapi.Object[projectRetentionResponse]()},
		{Path: "/api/v1/projects/{projectID}/retention", Method: http.MethodPut, Scope: auth.ScopeAdmin, Tags: []string{"projects"}, OperationID: "updateProjectRetention", RequestBody: true, SuccessStatus: http.StatusOK, Handler: handler.updateRetention, Request: openapi.TypeOf[projectRetentionRequest](), Response: openapi.Object[projectRetentionResponse]()},
		{Path: "/api/v1/projects/{projectID}/environments", Method: http.MethodPost, Scope: auth.ScopeAdmin, Tags: []string{"projects"}, OperationID: "createEnvironment", RequestBody: true, Handler: handler.createEnvironment, Request: openapi.TypeOf[createEnvironmentRequest](), Response: openapi.Object[environmentResponse]()},
	}
}

type createProjectRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

type renameProjectRequest struct {
	Name string `json:"name"`
}

type createEnvironmentRequest struct {
	Name string `json:"name"`
}

type createProjectResponse struct {
	Environment auth.Environment `json:"environment"`
	Project     auth.Project     `json:"project"`
}

type projectDetailResponse struct {
	Environments []auth.Environment `json:"environments"`
	Project      auth.Project       `json:"project"`
	Role         string             `json:"role"`
}

type projectResponse struct {
	Project auth.Project `json:"project"`
}

type environmentResponse struct {
	Environment auth.Environment `json:"environment"`
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	user, userError := handler.backend.GetUser(request.Context(), principal.UserID)
	if handler.errors.Identity(responseWriter, request, "get user", userError) {
		return
	}
	memberships, listError := api.VisibleProjects(request.Context(), handler.backend, user)
	if handler.errors.Identity(responseWriter, request, "list projects", listError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, map[string]any{"data": memberships})
}

func (handler *Handler) create(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload createProjectRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	// A regular account may own a handful of projects; a super administrator is not limited.
	ownedLimit := auth.MaxOwnedProjects
	if principal.PlatformAdmin {
		ownedLimit = 0
	}
	project, environment, createError := handler.backend.CreateProject(request.Context(), auth.CreateProjectParams{
		Slug: payload.Slug, Name: payload.Name, EnvironmentName: payload.Environment,
	}, principal.UserID, ownedLimit, principal.Actor())
	if handler.errors.Identity(responseWriter, request, "create project", createError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusCreated, createProjectResponse{Project: project, Environment: environment})
}

func (handler *Handler) get(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	project, projectError := handler.backend.GetProject(request.Context(), principal.ProjectID)
	if handler.errors.Identity(responseWriter, request, "get project", projectError) {
		return
	}
	environments, listError := handler.backend.ListEnvironments(request.Context(), principal.ProjectID)
	if handler.errors.Identity(responseWriter, request, "list environments", listError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, projectDetailResponse{Project: project, Role: principal.Role, Environments: environments})
}

func (handler *Handler) rename(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	var payload renameProjectRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	project, renameError := handler.backend.RenameProject(request.Context(), principal.ProjectID, payload.Name, principal.Actor())
	if handler.errors.Identity(responseWriter, request, "rename project", renameError) {
		return
	}
	handler.cache.ForgetProject(principal.ProjectID)
	render.WriteJSON(responseWriter, http.StatusOK, projectResponse{Project: project})
}

func (handler *Handler) createEnvironment(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	var payload createEnvironmentRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	environment, createError := handler.backend.CreateEnvironment(request.Context(), principal.ProjectID, payload.Name, principal.Actor())
	if handler.errors.Identity(responseWriter, request, "create environment", createError) {
		return
	}
	// Readers cache a project's environments briefly; this server sees the new one at once.
	handler.cache.ForgetEnvironments(principal.ProjectID)
	render.WriteJSON(responseWriter, http.StatusCreated, environmentResponse{Environment: environment})
}

type projectRetentionRequest struct {
	Days *int32 `json:"retention_days"`
}

type projectRetentionResponse struct {
	Days              *int32 `json:"retention_days"`
	DefaultAgeSeconds int64  `json:"default_age_seconds"`
	MaxDays           int64  `json:"max_days"`
}

func (handler *Handler) getRetention(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	days, err := handler.retention.GetProjectRetention(request.Context(), principal.ProjectID)
	if !handler.errors.Identity(responseWriter, request, "read project retention", err) {
		handler.writeRetention(responseWriter, days)
	}
}

func (handler *Handler) updateRetention(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	// An explicit null inherits the default. A missing field leaves the invalid sentinel intact.
	missing := int32(-1)
	payload := projectRetentionRequest{Days: &missing}
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	if days := payload.Days; days != nil && (*days < 1 || int64(*days) > telemetry.MaxRetentionDays) {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", "retention_days must be a positive supported number of days or null to inherit the default")
		return
	}
	if err := handler.retention.UpdateProjectRetention(request.Context(), principal.ProjectID, payload.Days, principal.Actor()); !handler.errors.Identity(responseWriter, request, "update project retention", err) {
		handler.writeRetention(responseWriter, payload.Days)
	}
}

func (handler *Handler) writeRetention(responseWriter http.ResponseWriter, days *int32) {
	defaultSeconds := int64(handler.defaultRetention / time.Second)
	if handler.defaultRetention > 0 {
		defaultSeconds = max(1, defaultSeconds)
	}
	render.WriteJSON(responseWriter, http.StatusOK, projectRetentionResponse{Days: days, DefaultAgeSeconds: defaultSeconds, MaxDays: telemetry.MaxRetentionDays})
}
