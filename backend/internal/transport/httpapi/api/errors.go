package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// DecodeBody decodes a JSON body with unknown fields rejected and answers 400 when it cannot.
func DecodeBody(responseWriter http.ResponseWriter, request *http.Request, payload any) bool {
	decodeError := render.DecodeJSONBody(request, payload)
	if decodeError == nil {
		return true
	}
	message := decodeError.Error()
	if errors.Is(decodeError, io.EOF) {
		message = "request body is required"
	}
	render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", message)
	return false
}

// Errors writes the error responses that need the server's logger.
type Errors struct {
	Logger *slog.Logger
}

// Internal logs the cause with the request's context, so the line carries its trace and span IDs,
// and answers with a generic error that reveals nothing about the cause.
func (errs Errors) Internal(responseWriter http.ResponseWriter, request *http.Request, operation string, cause error) {
	render.WriteInternalErrorContext(request.Context(), responseWriter, errs.Logger, operation, cause)
}

// Identity maps account and project errors to HTTP and reports whether it answered.
func (errs Errors) Identity(responseWriter http.ResponseWriter, request *http.Request, operation string, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, auth.ErrInvalidIdentity):
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", invalidIdentityMessage(operationError))
	case errors.Is(operationError, auth.ErrEmailExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "EMAIL_EXISTS", "an account with this email already exists")
	case errors.Is(operationError, auth.ErrEnvironmentExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "ENVIRONMENT_EXISTS", "this project already has an environment with that name")
	case errors.Is(operationError, auth.ErrProjectSlugExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "SLUG_EXISTS", "a project with this slug already exists")
	case errors.Is(operationError, auth.ErrUserNotFound):
		render.WriteNotFound(responseWriter, "user")
	case errors.Is(operationError, auth.ErrProjectNotFound):
		render.WriteNotFound(responseWriter, "project")
	case errors.Is(operationError, auth.ErrMemberNotFound):
		render.WriteNotFound(responseWriter, "member")
	case errors.Is(operationError, auth.ErrMemberExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "MEMBER_EXISTS", "this user is already a member of the project")
	case errors.Is(operationError, auth.ErrLastOwner):
		render.WriteAPIError(responseWriter, http.StatusConflict, "LAST_OWNER", "a project must keep at least one active owner")
	case errors.Is(operationError, auth.ErrLastPlatformAdmin):
		render.WriteAPIError(responseWriter, http.StatusConflict, "LAST_PLATFORM_ADMIN", "the super administrator cannot be disabled")
	case errors.Is(operationError, auth.ErrTooManyProjects):
		render.WriteAPIError(responseWriter, http.StatusConflict, "PROJECT_LIMIT", fmt.Sprintf("you already own %d projects, the limit for a regular account", auth.MaxOwnedProjects))
	case errors.Is(operationError, auth.ErrIngestTokenNotFound):
		render.WriteNotFound(responseWriter, "ingest token")
	case errors.Is(operationError, auth.ErrIngestTokenExists):
		render.WriteAPIError(responseWriter, http.StatusConflict, "TOKEN_NAME_EXISTS", "an ingest token with this name already exists")
	case errors.Is(operationError, auth.ErrForbiddenRoleChange):
		render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role cannot manage a member with this role")
	default:
		errs.Internal(responseWriter, request, operation, operationError)
	}
	return true
}

// invalidIdentityMessage keeps the validation detail and drops the operation prefixes wrapped around it.
func invalidIdentityMessage(validationError error) string {
	marker := auth.ErrInvalidIdentity.Error() + ": "
	message := validationError.Error()
	if index := strings.Index(message, marker); index >= 0 {
		return message[index+len(marker):]
	}
	return message
}
