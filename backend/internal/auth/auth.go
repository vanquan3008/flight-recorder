package auth

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrUnauthorized       = errors.New("unauthorized")
	ErrAPIKeyNotFound     = errors.New("API key not found")
	ErrAPIKeyNameExists   = errors.New("API key name already exists")
	ErrInvalidAPIKeyInput = errors.New("invalid API key input")
	ErrLastAdminAPIKey    = errors.New("cannot revoke the last admin API key")
)

var apiKeyNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

var allowedAPIKeyScopes = map[string]struct{}{
	"admin": {}, "ingest": {}, "read": {}, "replay": {}, "write": {},
}

type Principal struct {
	KeyID     string
	ProjectID string
	Scopes    []string
}

func (principal Principal) HasScope(wanted string) bool {
	for _, scope := range principal.Scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type IssuedAPIKey struct {
	APIKey APIKey
	Key    string
}

type CreateAPIKeyParams struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type APIKeyFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

type APIKeyEvent struct {
	ID         string    `json:"id"`
	APIKeyID   string    `json:"api_key_id"`
	ActorKeyID string    `json:"actor_key_id"`
	Action     string    `json:"action"`
	CreatedAt  time.Time `json:"created_at"`
}

type APIKeyEventFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

func NormalizeAPIKeyScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrInvalidAPIKeyInput)
	}
	normalized := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if _, allowed := allowedAPIKeyScopes[scope]; !allowed {
			return nil, fmt.Errorf("%w: scope %q is not allowed", ErrInvalidAPIKeyInput, scope)
		}
		if _, duplicate := seen[scope]; duplicate {
			return nil, fmt.Errorf("%w: duplicate scope %q", ErrInvalidAPIKeyInput, scope)
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func NormalizeCreateAPIKeyParams(params CreateAPIKeyParams, now time.Time) (CreateAPIKeyParams, error) {
	params.Name = strings.TrimSpace(params.Name)
	if !apiKeyNamePattern.MatchString(params.Name) {
		return CreateAPIKeyParams{}, fmt.Errorf("%w: name must be 1-64 letters, digits, spaces, dots, underscores, or hyphens", ErrInvalidAPIKeyInput)
	}
	scopes, scopeError := NormalizeAPIKeyScopes(params.Scopes)
	if scopeError != nil {
		return CreateAPIKeyParams{}, scopeError
	}
	if params.ExpiresAt != nil {
		expiresAt := params.ExpiresAt.UTC()
		if !expiresAt.After(now) {
			return CreateAPIKeyParams{}, fmt.Errorf("%w: expires_at must be in the future", ErrInvalidAPIKeyInput)
		}
		params.ExpiresAt = &expiresAt
	}
	params.Scopes = scopes
	return params, nil
}
