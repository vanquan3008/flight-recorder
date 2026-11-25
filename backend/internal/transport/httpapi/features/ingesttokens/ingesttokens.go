// Package ingesttokens serves the OTLP ingest tokens a project's telemetry senders use.
package ingesttokens

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

type Backend interface {
	CreateIngestToken(ctx context.Context, projectID string, actor auth.Actor, params auth.CreateIngestTokenParams, sign func(tokenID string, expiresAt time.Time) (string, error)) (auth.IssuedIngestToken, error)
	ListIngestTokens(context.Context, string, auth.IngestTokenFilter) ([]auth.IngestToken, error)
	RevokeIngestToken(ctx context.Context, projectID, tokenID string, actor auth.Actor) error
}

// Invalidator drops a revoked token from the ingest token cache at once.
type Invalidator interface {
	Invalidate(tokenID string)
}

type Handler struct {
	backend     Backend
	secret      []byte
	invalidator Invalidator
	errors      api.Errors
}

// New signs tokens with secret. invalidator may be nil.
func New(backend Backend, secret []byte, invalidator Invalidator, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, secret: secret, invalidator: invalidator, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/projects/{projectID}/ingest-tokens", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"projects", "ingest-tokens"}, OperationID: "listIngestTokens", Query: []string{"Limit", "Cursor"}, Handler: handler.list, Response: openapi.Page[auth.IngestToken]()},
		{Path: "/api/v1/projects/{projectID}/ingest-tokens", Method: http.MethodPost, Scope: auth.ScopeAdmin, Tags: []string{"projects", "ingest-tokens"}, OperationID: "createIngestToken", RequestBody: true, Handler: handler.create, Request: openapi.TypeOf[createIngestTokenRequest](), Response: openapi.Object[issuedIngestTokenResponse]()},
		{Path: "/api/v1/projects/{projectID}/ingest-tokens/{tokenID}", Method: http.MethodDelete, Scope: auth.ScopeAdmin, Tags: []string{"projects", "ingest-tokens"}, OperationID: "revokeIngestToken", Handler: handler.revoke, Response: openapi.Empty()},
	}
}

type createIngestTokenRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type issuedIngestTokenResponse struct {
	JWT   string           `json:"jwt"`
	Token auth.IngestToken `json:"token"`
}

// create issues the credential a telemetry sender uses for this project's OTLP endpoint. The JWT is
// returned once; the owner can list and revoke the token afterwards.
func (handler *Handler) create(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload createIngestTokenRequest
	if !api.DecodeBody(responseWriter, request, &payload) {
		return
	}
	params := auth.CreateIngestTokenParams{Name: payload.Name}
	if payload.ExpiresAt != nil {
		params.ExpiresAt = *payload.ExpiresAt
	}
	issued, createError := handler.backend.CreateIngestToken(request.Context(), principal.ProjectID, principal.Actor(), params,
		func(tokenID string, expiresAt time.Time) (string, error) {
			return auth.IssueIngestToken(handler.secret, principal.ProjectID, tokenID, time.Now(), expiresAt)
		})
	if handler.errors.Identity(responseWriter, request, "create ingest token", createError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusCreated, issuedIngestTokenResponse{Token: issued.Token, JWT: issued.JWT})
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !api.RequireProject(responseWriter, principal) {
		return
	}
	api.ListPage(responseWriter, request, func(page api.Page) ([]auth.IngestToken, error) {
		return handler.backend.ListIngestTokens(request.Context(), principal.ProjectID, auth.IngestTokenFilter{Before: page.Before, CursorID: page.CursorID, Limit: page.Limit})
	}, func(token auth.IngestToken) render.PageCursor {
		return render.PageCursor{At: token.CreatedAt, ID: token.ID}
	}, func(listError error) {
		handler.errors.Identity(responseWriter, request, "list ingest tokens", listError)
	})
}

func (handler *Handler) revoke(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	tokenID := request.PathValue("tokenID")
	if !api.RequireProject(responseWriter, principal) || !render.RequireUUID(responseWriter, tokenID) {
		return
	}
	if revokeError := handler.backend.RevokeIngestToken(request.Context(), principal.ProjectID, tokenID, principal.Actor()); handler.errors.Identity(responseWriter, request, "revoke ingest token", revokeError) {
		return
	}
	if handler.invalidator != nil {
		handler.invalidator.Invalidate(tokenID)
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}
