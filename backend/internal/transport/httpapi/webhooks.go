package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type webhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

func (server *Server) webhookBackend(responseWriter http.ResponseWriter) (WebhookBackend, bool) {
	if !server.webhooksEnabled {
		writeAPIError(responseWriter, http.StatusNotImplemented, "FEATURE_DISABLED", "webhook notifications are not enabled")
		return nil, false
	}
	backend, ok := server.backend.(WebhookBackend)
	if !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "FEATURE_DISABLED", "webhook notifications are not enabled")
		return nil, false
	}
	return backend, true
}

func (server *Server) listWebhooks(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", "cursor is invalid")
		return
	}
	webhooks, queryError := backend.ListWebhooks(request.Context(), principal.ProjectID, notificationdomain.WebhookFilter{Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if queryError != nil {
		server.logger.Error("list webhooks", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(webhooks, limit, func(webhook notificationdomain.Webhook) pageCursor {
		return pageCursor{At: webhook.CreatedAt, ID: webhook.ID}
	}))
}

func (server *Server) createWebhook(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	var payload webhookRequest
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		if errors.Is(decodeError, io.EOF) {
			decodeError = errors.New("request body must contain name, url, and events")
		}
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	issued, createError := backend.CreateWebhook(request.Context(), principal.ProjectID, principal.KeyID, notificationdomain.CreateWebhookParams{Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Secret: payload.Secret, Events: payload.Events})
	if writeWebhookError(responseWriter, createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, map[string]any{"webhook": issued.Webhook, "secret": issued.Secret})
}

func (server *Server) getWebhook(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	webhook, queryError := backend.GetWebhook(request.Context(), principal.ProjectID, webhookID)
	if writeWebhookError(responseWriter, queryError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, webhook)
}

func (server *Server) updateWebhook(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	var payload webhookRequest
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	updated, updateError := backend.UpdateWebhook(request.Context(), principal.ProjectID, webhookID, notificationdomain.UpdateWebhookParams{Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Events: payload.Events, Enabled: payload.Enabled})
	if writeWebhookError(responseWriter, updateError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, updated)
}

func (server *Server) deleteWebhook(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	deleted, deleteError := backend.DeleteWebhook(request.Context(), principal.ProjectID, webhookID)
	if writeWebhookError(responseWriter, deleteError) {
		return
	}
	if !deleted {
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "webhook not found")
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (server *Server) rotateWebhookSecret(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	secret, rotateError := backend.RotateWebhookSecret(request.Context(), principal.ProjectID, webhookID)
	if writeWebhookError(responseWriter, rotateError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, map[string]string{"secret": secret})
}

func (server *Server) listWebhookDeliveries(responseWriter http.ResponseWriter, request *http.Request) {
	principal, ok := server.authenticateScope(responseWriter, request, "admin")
	if !ok {
		return
	}
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return
	}
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_QUERY", "cursor is invalid")
		return
	}
	deliveries, queryError := backend.ListWebhookDeliveries(request.Context(), principal.ProjectID, notificationdomain.DeliveryFilter{WebhookID: webhookID, Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if queryError != nil {
		server.logger.Error("list webhook deliveries", "error", queryError)
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, pageResponse(deliveries, limit, func(delivery notificationdomain.Delivery) pageCursor {
		return pageCursor{At: delivery.CreatedAt, ID: delivery.ID}
	}))
}

func writeWebhookError(responseWriter http.ResponseWriter, operationError error) bool {
	if operationError == nil {
		return false
	}
	switch {
	case errors.Is(operationError, notificationdomain.ErrInvalidWebhook):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
	case errors.Is(operationError, notificationdomain.ErrNameExists):
		writeAPIError(responseWriter, http.StatusConflict, "WEBHOOK_NAME_EXISTS", "a webhook with this name already exists")
	case errors.Is(operationError, notificationdomain.ErrWebhookNotFound):
		writeAPIError(responseWriter, http.StatusNotFound, "NOT_FOUND", "webhook not found")
	default:
		writeAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
