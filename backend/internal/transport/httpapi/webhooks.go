package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

var webhookRoutes = []apiRouteDefinition{
	{path: "/api/v1/webhooks", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhooks", query: []string{"Limit", "Cursor"}, handler: (*Server).listWebhooks, response: openapi.Page[notificationdomain.Webhook]()},
	{path: "/api/v1/webhooks", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "createWebhook", requestBody: true, handler: (*Server).createWebhook, request: openapi.TypeOf[webhookRequest](), response: openapi.Object[createWebhookResponse]()},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "getWebhook", handler: (*Server).getWebhook, response: openapi.Object[notificationdomain.Webhook]()},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodPut, scope: "admin", tags: []string{"webhooks"}, operationID: "updateWebhook", requestBody: true, handler: (*Server).updateWebhook, request: openapi.TypeOf[webhookRequest](), response: openapi.Object[notificationdomain.Webhook]()},
	{path: "/api/v1/webhooks/{webhookID}", method: http.MethodDelete, scope: "admin", tags: []string{"webhooks"}, operationID: "deleteWebhook", handler: (*Server).deleteWebhook, response: openapi.Empty()},
	{path: "/api/v1/webhooks/{webhookID}/rotate", method: http.MethodPost, scope: "admin", tags: []string{"webhooks"}, operationID: "rotateWebhookSecret", successStatus: http.StatusOK, handler: (*Server).rotateWebhookSecret, response: openapi.Object[webhookSecretResponse]()},
	{path: "/api/v1/webhooks/{webhookID}/deliveries", method: http.MethodGet, scope: "admin", tags: []string{"webhooks"}, operationID: "listWebhookDeliveries", query: []string{"Limit", "Cursor"}, handler: (*Server).listWebhookDeliveries, response: openapi.Page[notificationdomain.Delivery]()},
}

type webhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

func (server *Server) webhookBackend(responseWriter http.ResponseWriter) (WebhookBackend, bool) {
	backend, ok := server.backend.(WebhookBackend)
	if !server.webhooksEnabled || !ok {
		writeAPIError(responseWriter, http.StatusNotImplemented, "FEATURE_DISABLED", "webhook notifications are not enabled")
		return nil, false
	}
	return backend, true
}

// webhookTarget resolves the webhookID path value and the webhook backend,
// answering the error itself when either is unusable.
func (server *Server) webhookTarget(responseWriter http.ResponseWriter, request *http.Request) (WebhookBackend, string, bool) {
	webhookID := request.PathValue("webhookID")
	if !requireUUID(responseWriter, webhookID) {
		return nil, "", false
	}
	backend, ok := server.webhookBackend(responseWriter)
	return backend, webhookID, ok
}

func (server *Server) listWebhooks(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, ok := server.webhookBackend(responseWriter)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	webhooks, queryError := backend.ListWebhooks(request.Context(), principal.ProjectID, notificationdomain.WebhookFilter{Before: cursor.At, CursorID: cursor.ID, Limit: limit})
	if queryError != nil {
		server.writeInternalError(responseWriter, "list webhooks", queryError)
		return
	}
	writePage(responseWriter, webhooks, limit, func(webhook notificationdomain.Webhook) pageCursor {
		return pageCursor{At: webhook.CreatedAt, ID: webhook.ID}
	})
}

func (server *Server) createWebhook(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
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
	issued, createError := backend.CreateWebhook(request.Context(), principal.ProjectID, principal.UserID, notificationdomain.CreateWebhookParams{
		Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Secret: payload.Secret, Events: payload.Events,
	})
	if server.writeWebhookError(responseWriter, "create webhook", createError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusCreated, createWebhookResponse{Webhook: issued.Webhook, Secret: issued.Secret})
}

func (server *Server) getWebhook(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, webhookID, ok := server.webhookTarget(responseWriter, request)
	if !ok {
		return
	}
	webhook, queryError := backend.GetWebhook(request.Context(), principal.ProjectID, webhookID)
	if server.writeWebhookError(responseWriter, "get webhook", queryError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, webhook)
}

func (server *Server) updateWebhook(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, webhookID, ok := server.webhookTarget(responseWriter, request)
	if !ok {
		return
	}
	var payload webhookRequest
	if decodeError := decodeJSONBody(request, &payload); decodeError != nil {
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	updated, updateError := backend.UpdateWebhook(request.Context(), principal.ProjectID, webhookID, notificationdomain.UpdateWebhookParams{
		Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Events: payload.Events, Enabled: payload.Enabled,
	})
	if server.writeWebhookError(responseWriter, "update webhook", updateError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, updated)
}

func (server *Server) deleteWebhook(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, webhookID, ok := server.webhookTarget(responseWriter, request)
	if !ok {
		return
	}
	deleted, deleteError := backend.DeleteWebhook(request.Context(), principal.ProjectID, webhookID)
	if server.writeWebhookError(responseWriter, "delete webhook", deleteError) {
		return
	}
	if !deleted {
		writeNotFound(responseWriter, "webhook")
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (server *Server) rotateWebhookSecret(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, webhookID, ok := server.webhookTarget(responseWriter, request)
	if !ok {
		return
	}
	secret, rotateError := backend.RotateWebhookSecret(request.Context(), principal.ProjectID, webhookID)
	if server.writeWebhookError(responseWriter, "rotate webhook secret", rotateError) {
		return
	}
	writeJSONValue(responseWriter, http.StatusOK, webhookSecretResponse{Secret: secret})
}

func (server *Server) listWebhookDeliveries(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	backend, webhookID, ok := server.webhookTarget(responseWriter, request)
	if !ok {
		return
	}
	limit, cursor, parseError := parsePagination(request, validUUID)
	if parseError != nil {
		writeInvalidQuery(responseWriter, parseError)
		return
	}
	deliveries, queryError := backend.ListWebhookDeliveries(request.Context(), principal.ProjectID, notificationdomain.DeliveryFilter{
		WebhookID: webhookID, Before: cursor.At, CursorID: cursor.ID, Limit: limit,
	})
	if queryError != nil {
		server.writeInternalError(responseWriter, "list webhook deliveries", queryError)
		return
	}
	writePage(responseWriter, deliveries, limit, func(delivery notificationdomain.Delivery) pageCursor {
		return pageCursor{At: delivery.CreatedAt, ID: delivery.ID}
	})
}

// writeWebhookError maps webhook domain errors to HTTP and reports whether it
// answered the request.
func (server *Server) writeWebhookError(responseWriter http.ResponseWriter, operation string, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, notificationdomain.ErrInvalidWebhook):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
	case errors.Is(operationError, notificationdomain.ErrNameExists):
		writeAPIError(responseWriter, http.StatusConflict, "WEBHOOK_NAME_EXISTS", "a webhook with this name already exists")
	case errors.Is(operationError, notificationdomain.ErrWebhookNotFound):
		writeNotFound(responseWriter, "webhook")
	default:
		server.writeInternalError(responseWriter, operation, operationError)
	}
	return true
}
