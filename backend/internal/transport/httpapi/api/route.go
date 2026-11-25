// Package api is the contract between the router and the feature packages: the route declaration,
// the handler signature, and the request and error helpers every feature shares.
package api

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/openapi"
)

const (
	HeaderProjectID   = "X-Project-ID"
	HeaderEnvironment = "X-Environment"
)

// ScopePublic marks a route that needs no credentials.
const ScopePublic = "public"

// BearerToken is the credential in the Authorization header, or "" without one.
func BearerToken(request *http.Request) string {
	authorization := request.Header.Get("Authorization")
	if len(authorization) > 7 && strings.EqualFold(authorization[:7], "Bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	return ""
}

// Handler serves one route for a caller the router has already admitted with the route's scope.
type Handler func(http.ResponseWriter, *http.Request, auth.Principal)

// Route declares one endpoint. The router admits requests from it, and the OpenAPI document and the
// CORS method list are generated from it, so none of them can miss an endpoint.
type Route struct {
	Path          string
	Method        string
	Scope         string
	Tags          []string
	OperationID   string
	SuccessStatus int
	RequestBody   bool
	Handler       Handler
	// Public routes need no credentials.
	Public bool
	// Streaming routes hold their connection open and are watched instead of timed out.
	Streaming bool
	// LogQuery routes read logs from ClickHouse and take slots from their own pool.
	LogQuery bool
	// Query and Headers name reusable OpenAPI parameters.
	Query    []string
	Headers  []string
	Request  reflect.Type
	Response openapi.Body
}
