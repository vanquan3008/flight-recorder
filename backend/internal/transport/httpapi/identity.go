package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const (
	loginAttemptsPerEmail = 10
	// maxEmailLength is the longest address auth.NormalizeEmail accepts.
	maxEmailLength = 254
)

// sessionView is what the console needs after signing in or reloading. The access token is present when
// the response mints one.
type sessionView struct {
	User        auth.User         `json:"user"`
	Memberships []auth.Membership `json:"memberships"`
	AccessToken string            `json:"access_token,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

func decodeIdentityBody(request *http.Request, payload any) error {
	if decodeError := decodeJSONBody(request, payload); decodeError != nil {
		if errors.Is(decodeError, io.EOF) {
			return errors.New("request body is required")
		}
		return decodeError
	}
	return nil
}

// writeIdentityError maps account and project errors to HTTP and reports whether it answered.
func (server *Server) writeIdentityError(responseWriter http.ResponseWriter, operation string, operationError error) bool {
	switch {
	case operationError == nil:
		return false
	case errors.Is(operationError, auth.ErrInvalidIdentity):
		writeAPIError(responseWriter, http.StatusBadRequest, "INVALID_REQUEST", invalidIdentityMessage(operationError))
	case errors.Is(operationError, auth.ErrEmailExists):
		writeAPIError(responseWriter, http.StatusConflict, "EMAIL_EXISTS", "an account with this email already exists")
	case errors.Is(operationError, auth.ErrEnvironmentExists):
		writeAPIError(responseWriter, http.StatusConflict, "ENVIRONMENT_EXISTS", "this project already has an environment with that name")
	case errors.Is(operationError, auth.ErrProjectSlugExists):
		writeAPIError(responseWriter, http.StatusConflict, "SLUG_EXISTS", "a project with this slug already exists")
	case errors.Is(operationError, auth.ErrUserNotFound):
		writeNotFound(responseWriter, "user")
	case errors.Is(operationError, auth.ErrProjectNotFound):
		writeNotFound(responseWriter, "project")
	case errors.Is(operationError, auth.ErrMemberNotFound):
		writeNotFound(responseWriter, "member")
	case errors.Is(operationError, auth.ErrMemberExists):
		writeAPIError(responseWriter, http.StatusConflict, "MEMBER_EXISTS", "this user is already a member of the project")
	case errors.Is(operationError, auth.ErrLastOwner):
		writeAPIError(responseWriter, http.StatusConflict, "LAST_OWNER", "a project must keep at least one active owner")
	case errors.Is(operationError, auth.ErrLastPlatformAdmin):
		writeAPIError(responseWriter, http.StatusConflict, "LAST_PLATFORM_ADMIN", "the platform needs at least one active administrator")
	case errors.Is(operationError, auth.ErrRegistrationClosed):
		writeAPIError(responseWriter, http.StatusForbidden, "REGISTRATION_CLOSED", "registration is closed; ask an administrator for an account")
	case errors.Is(operationError, auth.ErrTooManyProjects):
		writeAPIError(responseWriter, http.StatusConflict, "PROJECT_LIMIT", fmt.Sprintf("you already own %d projects, the limit for a regular account", auth.MaxOwnedProjects))
	case errors.Is(operationError, auth.ErrIngestTokenNotFound):
		writeNotFound(responseWriter, "ingest token")
	case errors.Is(operationError, auth.ErrIngestTokenExists):
		writeAPIError(responseWriter, http.StatusConflict, "TOKEN_NAME_EXISTS", "an ingest token with this name already exists")
	case errors.Is(operationError, auth.ErrForbiddenRoleChange):
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role cannot manage a member with this role")
	default:
		server.writeInternalError(responseWriter, operation, operationError)
	}
	return true
}

func (server *Server) projectsFor(request *http.Request, user auth.User) ([]auth.Membership, error) {
	if user.PlatformAdmin {
		return server.backend.ListAllProjects(request.Context())
	}
	return server.backend.ListMemberships(request.Context(), user.ID)
}

// requireProjectAccess confirms the project in the path is the one the router authorized the caller
// for; the router resolves the path's project and checks membership before any handler runs.
func requireProjectAccess(responseWriter http.ResponseWriter, request *http.Request, principal auth.Principal) bool {
	if principal.ProjectID != "" {
		return true
	}
	writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "no project selected")
	return false
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
