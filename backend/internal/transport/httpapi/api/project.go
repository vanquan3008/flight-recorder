package api

import (
	"context"
	"net/http"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// RequireProject confirms the router authorized the caller for a project. The router resolves the
// project in the path and checks membership before any handler runs.
func RequireProject(responseWriter http.ResponseWriter, principal auth.Principal) bool {
	if principal.ProjectID != "" {
		return true
	}
	render.WriteAPIError(responseWriter, http.StatusForbidden, "FORBIDDEN", "no project selected")
	return false
}

// MembershipLister lists the projects an account can open.
type MembershipLister interface {
	ListAllProjects(context.Context) ([]auth.Membership, error)
	ListMemberships(context.Context, string) ([]auth.Membership, error)
}

// VisibleProjects lists every project for the super administrator and the account's own projects
// for everyone else.
func VisibleProjects(ctx context.Context, lister MembershipLister, user auth.User) ([]auth.Membership, error) {
	if user.PlatformAdmin {
		return lister.ListAllProjects(ctx)
	}
	return lister.ListMemberships(ctx, user.ID)
}
