package httpapi

import (
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

// createIngestToken issues the credential a telemetry sender uses for this project's OTLP endpoint. The
// JWT is returned once; the owner can list and revoke the token afterwards.
func (server *Server) createIngestToken(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	var payload struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
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
			return auth.IssueIngestToken(server.jwtSecret, principal.ProjectID, tokenID, time.Now(), expiresAt)
		})
	if server.writeIdentityError(responseWriter, "create ingest token", createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"token": issued.Token, "jwt": issued.JWT})
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
	responseWriter.WriteHeader(http.StatusNoContent)
}
