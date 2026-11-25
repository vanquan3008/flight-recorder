package otlp

import (
	"context"
	"errors"
	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

// ErrUnknownEnvironment reports an environment the token's project does not have.
var ErrUnknownEnvironment = errors.New("unknown environment")

// ScopeResolver names where a token's telemetry is stored: the project it belongs to and the
// environment the sender asked for (empty for the project's default).
type ScopeResolver func(ctx context.Context, projectID, environment string) (config.IngestionConfig, error)

type scopeContextKey struct{}

// withScope carries the scope an authorized request resolved to into its export.
func withScope(ctx context.Context, scope config.IngestionConfig) context.Context {
	return context.WithValue(ctx, scopeContextKey{}, scope)
}

// WithScopes lets one receiver serve many projects: the project comes from the ingest token and the
// environment from the sender. Without it the receiver serves only the configured project.
func (receiver *Receiver) WithScopes(resolve ScopeResolver) *Receiver {
	receiver.scopes = resolve
	return receiver
}

// normalizerFor returns the normalizer for the scope the request was authorized into.
func (receiver *Receiver) normalizerFor(ctx context.Context) *normalize.Normalizer {
	if scope, scoped := ctx.Value(scopeContextKey{}).(config.IngestionConfig); scoped {
		return normalize.New(scope)
	}
	return receiver.normalizer
}

// authorizeScope checks the ingest token and resolves where the request's telemetry goes. client
// identifies the caller for failure accounting; environment is what the sender asked for, and an
// empty value means the project's default. It returns auth.ErrUnauthorized for a bad token, a
// throttled error while the client is over its failure budget, and any other error for an
// authentication outage, which callers must treat as retryable. The returned context carries the
// scope to the export.
func (receiver *Receiver) authorizeScope(ctx context.Context, rawKey, environment, client string) (context.Context, error) {
	if receiver.authenticate == nil {
		return ctx, nil
	}
	if retryAfter, blocked := receiver.guard.throttled(client); blocked {
		return ctx, &throttledError{retryAfter: retryAfter}
	}
	if receiver.guard.knownBad(rawKey) {
		receiver.guard.failed(client, rawKey, false)
		return ctx, auth.ErrUnauthorized
	}
	principal, authError := receiver.authenticate(ctx, rawKey)
	if authError != nil {
		if errors.Is(authError, auth.ErrUnauthorized) {
			receiver.guard.failed(client, rawKey, true)
		}
		return ctx, authError
	}
	if !principal.HasScope("ingest") {
		receiver.guard.failed(client, rawKey, false)
		return ctx, auth.ErrUnauthorized
	}
	if receiver.scopes == nil {
		if principal.ProjectID != receiver.projectID {
			receiver.guard.failed(client, rawKey, false)
			return ctx, auth.ErrUnauthorized
		}
		return ctx, nil
	}
	scope, scopeError := receiver.scopes(ctx, principal.ProjectID, environment)
	if scopeError != nil {
		return ctx, scopeError
	}
	return withScope(ctx, scope), nil
}
