package httpapi

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const streamAccessCheckInterval = 15 * time.Second

// Streams expire with their JWT. Recheck account and membership directly, so
// a revoked permission is not kept alive by an open connection or a role cache.
func (server *Server) watchStreamAccess(ctx context.Context, principal auth.Principal, claims auth.Claims) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithDeadline(ctx, claims.ExpiresAt)
	go func() {
		defer cancel()
		ticker := time.NewTicker(streamAccessCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				allowed := server.streamAccessAllowed(checkCtx, principal, claims)
				stop()
				if !allowed {
					return
				}
			}
		}
	}()
	return ctx, cancel
}

func (server *Server) streamAccessAllowed(ctx context.Context, principal auth.Principal, claims auth.Claims) bool {
	user, err := server.backend.GetUser(ctx, principal.UserID)
	if err != nil || user.ID != claims.Subject || user.DisabledAt != nil || user.MustChangePassword ||
		claims.IssuedAt.Unix() < user.TokensValidAfter.Unix() || !time.Now().Before(claims.ExpiresAt) {
		return false
	}
	if user.PlatformAdmin {
		return true
	}
	role, err := server.backend.MemberRole(ctx, principal.ProjectID, principal.UserID)
	return err == nil && (auth.Principal{Scopes: auth.ScopesForRole(role)}).HasScope(auth.ScopeRead)
}
