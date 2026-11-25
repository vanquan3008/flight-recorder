package access

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// Authenticate identifies the caller from their access token and checks the scope, answering the
// error itself when it fails. It serves endpoints outside the route table, such as /metrics.
func (guard *Guard) Authenticate(responseWriter http.ResponseWriter, request *http.Request, scope string) (auth.Principal, bool) {
	principal, _, ok := guard.authenticate(responseWriter, request, scope)
	return principal, ok
}

// authenticate also returns the verified token claims, which a stream needs to end the connection
// when the token expires. A person acts inside the project they name, with the scopes their role
// grants. The token only says who the person is: whether the account is still active, a super
// administrator, or a member of the project is read from the database each time.
func (guard *Guard) authenticate(responseWriter http.ResponseWriter, request *http.Request, scope string) (auth.Principal, auth.Claims, bool) {
	raw := api.BearerToken(request)
	if raw == "" {
		responseWriter.Header().Set("WWW-Authenticate", "Bearer")
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, auth.Claims{}, false
	}
	claims, verifyError := auth.VerifyJWTWithIssuerAudience(guard.tokenSecret, raw, auth.TokenAccess, guard.tokenIssuer, guard.tokenAudience, time.Now())
	if verifyError != nil {
		responseWriter.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		// A distinct code lets the console refresh instead of sending the person back to sign in.
		code := "UNAUTHORIZED"
		if errors.Is(verifyError, auth.ErrExpiredToken) {
			code = "TOKEN_EXPIRED"
		}
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, code, "authentication required")
		return auth.Principal{}, auth.Claims{}, false
	}
	user, userError := guard.store.GetUser(request.Context(), claims.Subject)
	if errors.Is(userError, auth.ErrUserNotFound) {
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, auth.Claims{}, false
	}
	if userError != nil {
		guard.errors.Internal(responseWriter, request, "load user", userError)
		return auth.Principal{}, auth.Claims{}, false
	}
	// Disabling an account, or changing its password, ends the tokens issued before it.
	if user.DisabledAt != nil || claims.TokenVersion != user.TokenVersion() {
		render.WriteAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, auth.Claims{}, false
	}
	principal, ok := guard.principalFor(responseWriter, request, scope, user)
	return principal, claims, ok
}

func allowedWithTemporaryPassword(path string) bool {
	return path == "/api/v1/auth/me" || path == "/api/v1/auth/password"
}

func (guard *Guard) principalFor(responseWriter http.ResponseWriter, request *http.Request, scope string, user auth.User) (auth.Principal, bool) {
	// An account on a temporary password may only look at itself and replace the password. This holds for every
	// route, including those open to any signed-in person and those for super administrators.
	if user.MustChangePassword && !allowedWithTemporaryPassword(request.URL.Path) {
		render.WriteAPIError(responseWriter, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "change your temporary password before continuing")
		return auth.Principal{}, false
	}
	principal := auth.Principal{UserID: user.ID, PlatformAdmin: user.PlatformAdmin, MustChangePassword: user.MustChangePassword, Scopes: []string{auth.ScopeSession}}
	if user.PlatformAdmin {
		principal.Scopes = append(principal.Scopes, auth.ScopePlatform)
	}
	switch scope {
	case auth.ScopeSession:
		return principal, true
	case auth.ScopePlatform:
		if !user.PlatformAdmin {
			render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "super administrator access is required")
			return auth.Principal{}, false
		}
		return principal, true
	}
	reference := request.PathValue("projectID")
	if reference == "" {
		reference = strings.TrimSpace(request.Header.Get(api.HeaderProjectID))
	}
	if reference == "" {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "PROJECT_REQUIRED", "choose a project with the "+api.HeaderProjectID+" header")
		return auth.Principal{}, false
	}
	now := time.Now()
	project, found := guard.projectFor(responseWriter, request, reference, now)
	if !found {
		return auth.Principal{}, false
	}
	role := auth.RoleOwner
	if !user.PlatformAdmin {
		if role, found = guard.roleFor(responseWriter, request, project.ID, user.ID, now); !found {
			return auth.Principal{}, false
		}
	}
	principal.ProjectID, principal.Role = project.ID, role
	principal.Scopes = append(principal.Scopes, auth.ScopesForRole(role)...)
	if !principal.HasScope(scope) {
		render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role in this project does not allow this action")
		return auth.Principal{}, false
	}
	return principal, true
}

// projectFor loads the project a request names, by ID or slug, through the project cache.
func (guard *Guard) projectFor(responseWriter http.ResponseWriter, request *http.Request, reference string, now time.Time) (auth.Project, bool) {
	if project, cached := guard.projects.get(reference, now); cached {
		guard.metrics.ObserveAuthCache("project", "hit")
		return project, true
	}
	guard.metrics.ObserveAuthCache("project", "miss")
	project, projectError := guard.store.GetProject(request.Context(), reference)
	if errors.Is(projectError, auth.ErrProjectNotFound) {
		// The same answer as for a project the caller cannot open, so names cannot be probed.
		render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "you do not have access to this project")
		return auth.Project{}, false
	}
	if projectError != nil {
		guard.errors.Internal(responseWriter, request, "get project", projectError)
		return auth.Project{}, false
	}
	guard.projects.put(project, now, reference, project.ID, project.Slug)
	return project, true
}

// roleFor loads a member's role in a project through the role cache. A non-member has an empty role.
func (guard *Guard) roleFor(responseWriter http.ResponseWriter, request *http.Request, projectID, userID string, now time.Time) (string, bool) {
	key := roleCacheKey(projectID, userID)
	if role, cached := guard.roles.get(key, now); cached {
		guard.metrics.ObserveAuthCache("role", "hit")
		return role, true
	}
	guard.metrics.ObserveAuthCache("role", "miss")
	role, roleError := guard.store.MemberRole(request.Context(), projectID, userID)
	if roleError != nil {
		guard.errors.Internal(responseWriter, request, "get member role", roleError)
		return "", false
	}
	guard.roles.put(role, now, key)
	return role, true
}
