package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const (
	HeaderProjectID   = "X-Project-ID"
	HeaderEnvironment = "X-Environment"

	projectHeader     = HeaderProjectID
	environmentHeader = HeaderEnvironment
)

func bearerToken(request *http.Request) string {
	authorization := request.Header.Get("Authorization")
	if len(authorization) > 7 && strings.EqualFold(authorization[:7], "Bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	return ""
}

// authenticateScope identifies the caller from their access token and checks the route's scope.
// A person acts inside the project they name, with the scopes their role grants, so every handler
// sees the same Principal. The token only says who the person is: whether the account is still
// active, a super administrator, or a member of the project is read from the database each time.
func (server *Server) authenticateScope(responseWriter http.ResponseWriter, request *http.Request, scope string) (auth.Principal, bool) {
	raw := bearerToken(request)
	if raw == "" {
		responseWriter.Header().Set("WWW-Authenticate", "Bearer")
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, false
	}
	claims, verifyError := auth.VerifyJWTWithIssuerAudience(server.jwtSecret, raw, auth.TokenAccess, server.issuer, server.audience, time.Now())
	if verifyError != nil {
		responseWriter.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		// A distinct code lets the console refresh instead of sending the person back to sign in.
		code := "UNAUTHORIZED"
		if errors.Is(verifyError, auth.ErrExpiredToken) {
			code = "TOKEN_EXPIRED"
		}
		writeAPIError(responseWriter, http.StatusUnauthorized, code, "authentication required")
		return auth.Principal{}, false
	}
	user, userError := server.backend.GetUser(request.Context(), claims.Subject)
	if errors.Is(userError, auth.ErrUserNotFound) {
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, false
	}
	if userError != nil {
		server.writeInternalError(responseWriter, "load user", userError)
		return auth.Principal{}, false
	}
	// Disabling an account, or changing its password, ends the tokens issued before it.
	if user.DisabledAt != nil || claims.TokenVersion != user.TokenVersion() {
		writeAPIError(responseWriter, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return auth.Principal{}, false
	}
	return server.principalFor(responseWriter, request, scope, user)
}

func allowedWithTemporaryPassword(path string) bool {
	return path == "/api/v1/auth/me" || path == "/api/v1/auth/password"
}

func (server *Server) principalFor(responseWriter http.ResponseWriter, request *http.Request, scope string, user auth.User) (auth.Principal, bool) {
	// An account on a temporary password may only look at itself and replace the password. This holds for every
	// route, including those open to any signed-in person and those for super administrators.
	if user.MustChangePassword && !allowedWithTemporaryPassword(request.URL.Path) {
		writeAPIError(responseWriter, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "change your temporary password before continuing")
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
			writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "super administrator access is required")
			return auth.Principal{}, false
		}
		return principal, true
	}
	reference := request.PathValue("projectID")
	if reference == "" {
		reference = strings.TrimSpace(request.Header.Get(projectHeader))
	}
	if reference == "" {
		writeAPIError(responseWriter, http.StatusBadRequest, "PROJECT_REQUIRED", "choose a project with the "+projectHeader+" header")
		return auth.Principal{}, false
	}
	now := time.Now()
	var (
		project       auth.Project
		projectError  error
		cachedProject bool
	)
	if server.identity != nil {
		project, cachedProject = server.identity.getProject(reference, now)
	}
	if cachedProject {
		server.metrics.ObserveAuthCache("project", "hit")
	} else {
		server.metrics.ObserveAuthCache("project", "miss")
		project, projectError = server.backend.GetProject(request.Context(), reference)
		if errors.Is(projectError, auth.ErrProjectNotFound) {
			// The same answer as for a project the caller cannot open, so names cannot be probed.
			writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "you do not have access to this project")
			return auth.Principal{}, false
		}
		if projectError != nil {
			server.writeInternalError(responseWriter, "get project", projectError)
			return auth.Principal{}, false
		}
		if server.identity != nil {
			server.identity.putProject(reference, project, now)
		}
	}
	role := auth.RoleOwner
	if !user.PlatformAdmin {
		var roleError error
		cachedRole := false
		if server.identity != nil {
			role, cachedRole = server.identity.getRole(project.ID, user.ID, now)
		}
		if cachedRole {
			server.metrics.ObserveAuthCache("role", "hit")
		} else {
			server.metrics.ObserveAuthCache("role", "miss")
			if role, roleError = server.backend.MemberRole(request.Context(), project.ID, user.ID); roleError != nil {
				server.writeInternalError(responseWriter, "get member role", roleError)
				return auth.Principal{}, false
			}
			if server.identity != nil {
				server.identity.putRole(project.ID, user.ID, role, now)
			}
		}
	}
	principal.ProjectID, principal.Role = project.ID, role
	principal.Scopes = append(principal.Scopes, auth.ScopesForRole(role)...)
	if !principal.HasScope(scope) {
		writeAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "your role in this project does not allow this action")
		return auth.Principal{}, false
	}
	return principal, true
}

var errEnvironmentNotFound = errors.New("environment not found")

const (
	defaultScopeCacheTTL   = 30 * time.Second
	defaultScopeCacheLimit = 1024
)

type cachedEnvironments struct {
	environments []auth.Environment
	loadedAt     time.Time
}

// environmentCache keeps each project's environments briefly so reads do not look them up per request.
type environmentCache struct {
	mu      sync.Mutex
	entries map[string]cachedEnvironments
	ttl     time.Duration
	limit   int
}

func newEnvironmentCache(ttl time.Duration, limit int) *environmentCache {
	if ttl <= 0 {
		ttl = defaultScopeCacheTTL
	}
	if limit <= 0 {
		limit = defaultScopeCacheLimit
	}
	return &environmentCache{entries: make(map[string]cachedEnvironments), ttl: ttl, limit: limit}
}

func (cache *environmentCache) get(projectID string, now time.Time) ([]auth.Environment, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[projectID]
	return entry.environments, ok && now.Sub(entry.loadedAt) < cache.ttl
}

func (cache *environmentCache) forget(projectID string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.entries, projectID)
}

func (cache *environmentCache) put(projectID string, environments []auth.Environment, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) >= cache.limit {
		cache.entries = make(map[string]cachedEnvironments)
	}
	cache.entries[projectID] = cachedEnvironments{environments: environments, loadedAt: now}
}

// resolveEnvironment picks the environment a project request reads: the one named by the
// X-Environment header (ID or name), else the project's first. The project the server was
// configured with keeps its configured environment without a lookup.
func (server *Server) resolveEnvironment(ctx context.Context, projectID, reference string) (string, error) {
	if reference == "" && projectID == server.defaultProjectID && server.environmentID != "" {
		return server.environmentID, nil
	}
	now := time.Now()
	environments, cached := server.environments.get(projectID, now)
	if !cached {
		var listError error
		if environments, listError = server.backend.ListEnvironments(ctx, projectID); listError != nil {
			return "", listError
		}
		server.environments.put(projectID, environments, now)
	}
	if reference == "" {
		if len(environments) == 0 {
			return "", errEnvironmentNotFound
		}
		return environments[0].ID, nil
	}
	for _, environment := range environments {
		if environment.ID == reference || environment.Name == reference {
			return environment.ID, nil
		}
	}
	return "", errEnvironmentNotFound
}
