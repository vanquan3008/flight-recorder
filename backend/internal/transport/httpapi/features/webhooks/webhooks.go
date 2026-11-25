// Package webhooks serves a project's webhook endpoints and their deliveries.
package webhooks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// Backend is optional, so deployments can run without outbound notifications.
type Backend interface {
	ListWebhooks(context.Context, string, notificationdomain.WebhookFilter) ([]notificationdomain.Webhook, error)
	GetWebhook(context.Context, string, string) (notificationdomain.Webhook, error)
	CreateWebhook(context.Context, string, string, notificationdomain.CreateWebhookParams) (notificationdomain.IssuedWebhook, error)
	UpdateWebhook(context.Context, string, string, notificationdomain.UpdateWebhookParams) (notificationdomain.Webhook, error)
	DeleteWebhook(context.Context, string, string) (bool, error)
	RotateWebhookSecret(context.Context, string, string) (string, error)
	ListWebhookDeliveries(context.Context, string, notificationdomain.DeliveryFilter) ([]notificationdomain.Delivery, error)
}

type Handler struct {
	backend Backend
	errors  api.Errors
}

// New answers 501 on every route when backend is nil, which is how disabled notifications are served.
func New(backend Backend, logger *slog.Logger) *Handler {
	return &Handler{backend: backend, errors: api.Errors{Logger: logger}}
}

func (handler *Handler) Routes() []api.Route {
	return []api.Route{
		{Path: "/api/v1/webhooks", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "listWebhooks", Query: []string{"Limit", "Cursor"}, Handler: handler.list, Response: openapi.Page[notificationdomain.Webhook]()},
		{Path: "/api/v1/webhooks", Method: http.MethodPost, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "createWebhook", RequestBody: true, Handler: handler.create, Request: openapi.TypeOf[webhookRequest](), Response: openapi.Object[createWebhookResponse]()},
		{Path: "/api/v1/webhooks/{webhookID}", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "getWebhook", Handler: handler.get, Response: openapi.Object[notificationdomain.Webhook]()},
		{Path: "/api/v1/webhooks/{webhookID}", Method: http.MethodPut, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "updateWebhook", RequestBody: true, Handler: handler.update, Request: openapi.TypeOf[webhookRequest](), Response: openapi.Object[notificationdomain.Webhook]()},
		{Path: "/api/v1/webhooks/{webhookID}", Method: http.MethodDelete, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "deleteWebhook", Handler: handler.delete, Response: openapi.Empty()},
		{Path: "/api/v1/webhooks/{webhookID}/rotate", Method: http.MethodPost, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "rotateWebhookSecret", SuccessStatus: http.StatusOK, Handler: handler.rotateSecret, Response: openapi.Object[webhookSecretResponse]()},
		{Path: "/api/v1/webhooks/{webhookID}/deliveries", Method: http.MethodGet, Scope: auth.ScopeAdmin, Tags: []string{"webhooks"}, OperationID: "listWebhookDeliveries", Query: []string{"Limit", "Cursor"}, Handler: handler.listDeliveries, Response: openapi.Page[notificationdomain.Delivery]()},
	}
}

type webhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

type createWebhookResponse struct {
	Secret  string                     `json:"secret"`
	Webhook notificationdomain.Webhook `json:"webhook"`
}

type webhookSecretResponse struct {
	Secret string `json:"secret"`
}

func (handler *Handler) enabled(responseWriter http.ResponseWriter) bool {
	if handler.backend == nil {
		render.WriteAPIError(responseWriter, http.StatusNotImplemented, "FEATURE_DISABLED", "webhook notifications are not enabled")
		return false
	}
	return true
}

// target resolves the webhookID path value, answering the error itself when it or the backend is unusable.
func (handler *Handler) target(responseWriter http.ResponseWriter, request *http.Request) (string, bool) {
	webhookID := request.PathValue("webhookID")
	if !render.RequireUUID(responseWriter, webhookID) {
		return "", false
	}
	return webhookID, handler.enabled(responseWriter)
}

func (handler *Handler) list(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !handler.enabled(responseWriter) {
		return
	}
	api.ListPage(responseWriter, request, func(page api.Page) ([]notificationdomain.Webhook, error) {
		return handler.backend.ListWebhooks(request.Context(), principal.ProjectID, notificationdomain.WebhookFilter{Before: page.Before, CursorID: page.CursorID, Limit: page.Limit})
	}, func(webhook notificationdomain.Webhook) render.PageCursor {
		return render.PageCursor{At: webhook.CreatedAt, ID: webhook.ID}
	}, func(queryError error) { handler.errors.Internal(responseWriter, request, "list webhooks", queryError) })
}

func (handler *Handler) create(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	if !handler.enabled(responseWriter) {
		return
	}
	var payload webhookRequest
	if decodeError := render.DecodeJSONBody(request, &payload); decodeError != nil {
		if errors.Is(decodeError, io.EOF) {
			decodeError = errors.New("request body must contain name, url, and events")
		}
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	issued, createError := handler.backend.CreateWebhook(request.Context(), principal.ProjectID, principal.UserID, notificationdomain.CreateWebhookParams{
		Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Secret: payload.Secret, Events: payload.Events,
	})
	if handler.writeError(responseWriter, request, "create webhook", createError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusCreated, createWebhookResponse{Webhook: issued.Webhook, Secret: issued.Secret})
}

func (handler *Handler) get(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	webhookID, ok := handler.target(responseWriter, request)
	if !ok {
		return
	}
	webhook, queryError := handler.backend.GetWebhook(request.Context(), principal.ProjectID, webhookID)
	if handler.writeError(responseWriter, request, "get webhook", queryError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, webhook)
}

func (handler *Handler) update(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	webhookID, ok := handler.target(responseWriter, request)
	if !ok {
		return
	}
	var payload webhookRequest
	if decodeError := render.DecodeJSONBody(request, &payload); decodeError != nil {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", decodeError.Error())
		return
	}
	updated, updateError := handler.backend.UpdateWebhook(request.Context(), principal.ProjectID, webhookID, notificationdomain.UpdateWebhookParams{
		Name: strings.TrimSpace(payload.Name), URL: strings.TrimSpace(payload.URL), Events: payload.Events, Enabled: payload.Enabled,
	})
	if handler.writeError(responseWriter, request, "update webhook", updateError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, updated)
}

func (handler *Handler) delete(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	webhookID, ok := handler.target(responseWriter, request)
	if !ok {
		return
	}
	deleted, deleteError := handler.backend.DeleteWebhook(request.Context(), principal.ProjectID, webhookID)
	if handler.writeError(responseWriter, request, "delete webhook", deleteError) {
		return
	}
	if !deleted {
		render.WriteNotFound(responseWriter, "webhook")
		return
	}
	responseWriter.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) rotateSecret(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	webhookID, ok := handler.target(responseWriter, request)
	if !ok {
		return
	}
	secret, rotateError := handler.backend.RotateWebhookSecret(request.Context(), principal.ProjectID, webhookID)
	if handler.writeError(responseWriter, request, "rotate webhook secret", rotateError) {
		return
	}
	render.WriteJSON(responseWriter, http.StatusOK, webhookSecretResponse{Secret: secret})
}

func (handler *Handler) listDeliveries(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) {
	webhookID, ok := handler.target(responseWriter, request)
	if !ok {
		return
	}
	api.ListPage(responseWriter, request, func(page api.Page) ([]notificationdomain.Delivery, error) {
		return handler.backend.ListWebhookDeliveries(request.Context(), principal.ProjectID, notificationdomain.DeliveryFilter{
			WebhookID: webhookID, Before: page.Before, CursorID: page.CursorID, Limit: page.Limit,
		})
	}, func(delivery notificationdomain.Delivery) render.PageCursor {
		return render.PageCursor{At: delivery.CreatedAt, ID: delivery.ID}
	}, func(queryError error) {
		handler.errors.Internal(responseWriter, request, "list webhook deliveries", queryError)
	})
}

// writeError maps webhook errors to HTTP and reports whether it answered the request.
func (handler *Handler) writeError(responseWriter http.ResponseWriter, request *http.Request, operation string, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, notificationdomain.ErrInvalidWebhook):
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", operationError.Error())
	case errors.Is(operationError, notificationdomain.ErrNameExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "WEBHOOK_NAME_EXISTS", "a webhook with this name already exists")
	case errors.Is(operationError, notificationdomain.ErrWebhookNotFound):
		render.WriteNotFound(responseWriter, "webhook")
	default:
		handler.errors.Internal(responseWriter, request, operation, operationError)
	}
	return true
}
