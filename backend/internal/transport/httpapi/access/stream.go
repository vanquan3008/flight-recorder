package access

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

const streamCheckInterval = 15 * time.Second

// watchStream ends a stream when its token expires, and rechecks the account and membership
// directly, so a revoked permission is not kept alive by an open connection or the role cache.
func (guard *Guard) watchStream(ctx context.Context, principal auth.Principal, claims auth.Claims) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithDeadline(ctx, claims.ExpiresAt)
	go func() {
		defer cancel()
		ticker := time.NewTicker(streamCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				allowed := guard.streamAllowed(checkCtx, principal, claims)
				stop()
				if !allowed {
					return
				}
			}
		}
	}()
	return ctx, cancel
}

func (guard *Guard) streamAllowed(ctx context.Context, principal auth.Principal, claims auth.Claims) bool {
	user, err := guard.store.GetUser(ctx, principal.UserID)
	if err != nil || user.ID != claims.Subject || user.DisabledAt != nil || user.MustChangePassword ||
		claims.TokenVersion != user.TokenVersion() || !time.Now().Before(claims.ExpiresAt) {
		return false
	}
	if user.PlatformAdmin {
		return true
	}
	role, err := guard.store.MemberRole(ctx, principal.ProjectID, principal.UserID)
	return err == nil && (auth.Principal{Scopes: auth.ScopesForRole(role)}).HasScope(auth.ScopeRead)
}
