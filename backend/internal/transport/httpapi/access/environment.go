package access

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

var errEnvironmentNotFound = errors.New("environment not found")

// selectEnvironment fills in the environment a project request reads, from the X-Environment header
// or the project's first environment. Requests outside a project need none.
func (guard *Guard) selectEnvironment(responseWriter http.ResponseWriter, request *http.Request, principal *auth.Principal) bool {
	if principal.ProjectID == "" {
		return true
	}
	reference := strings.TrimSpace(request.Header.Get(api.HeaderEnvironment))
	environmentID, environmentError := guard.resolveEnvironment(request.Context(), principal.ProjectID, reference)
	if errors.Is(environmentError, errEnvironmentNotFound) {
		render.WriteAPIError(responseWriter, http.StatusBadRequest, "INVALID_ENVIRONMENT", "unknown environment for this project")
		return false
	}
	if environmentError != nil {
		guard.errors.Internal(responseWriter, request, "resolve environment", environmentError)
		return false
	}
	principal.EnvironmentID = environmentID
	return true
}

// resolveEnvironment picks the environment named by ID or name, else the project's first. The
// configured project keeps its configured environment without a lookup.
func (guard *Guard) resolveEnvironment(ctx context.Context, projectID, reference string) (string, error) {
	if reference == "" && projectID == guard.defaultProjectID && guard.defaultEnvironmentID != "" {
		return guard.defaultEnvironmentID, nil
	}
	now := time.Now()
	environments, cached := guard.environments.get(projectID, now)
	if !cached {
		var listError error
		if environments, listError = guard.store.ListEnvironments(ctx, projectID); listError != nil {
			return "", listError
		}
		guard.environments.put(environments, now, projectID)
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
