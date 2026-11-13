package auth

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
)

// Scopes name what a route needs. A signed-in person gets them from their role in a project; a
// telemetry sender holds only ScopeIngest.
const (
	ScopeRead   = "read"
	ScopeWrite  = "write"
	ScopeReplay = "replay"
	ScopeAdmin  = "admin"
	ScopeIngest = "ingest"
)

// Principal is an authenticated caller: a signed-in user acting inside one project, or a telemetry
// sender holding an ingest token. Handlers read the project from here, never from the client.
type Principal struct {
	ProjectID string
	Scopes    []string
	// UserID, Role and PlatformAdmin are set for a signed-in user. A user with
	// MustChangePassword may only change it.
	UserID             string
	Role               string
	PlatformAdmin      bool
	MustChangePassword bool
	// TokenID names the ingest token a sender presented.
	TokenID string
	// EnvironmentID is the environment the request reads; the router sets it for project routes.
	EnvironmentID string
}

func (principal Principal) HasScope(wanted string) bool {
	for _, scope := range principal.Scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

// Actor names who performed an audited change. It is empty when TracePilot acted itself,
// such as reopening an incident.
type Actor struct {
	UserID string
}

func (actor Actor) IsZero() bool { return actor.UserID == "" }

// Label is the stable text form kept where a column cannot hold a foreign key.
func (actor Actor) Label() string {
	if actor.UserID == "" {
		return "system"
	}
	return "user:" + actor.UserID
}

func (principal Principal) Actor() Actor {
	return Actor{UserID: principal.UserID}
}

// IngestToken is a telemetry sender's credential as listed to an owner; the signed token itself is
// shown once, when it is created.
type IngestToken struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id,omitempty"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the token can still be used at the given moment.
func (token IngestToken) Active(now time.Time) bool {
	return token.RevokedAt == nil && now.Before(token.ExpiresAt)
}

type IssuedIngestToken struct {
	Token IngestToken
	// JWT is the signed credential, returned once.
	JWT string
}

type CreateIngestTokenParams struct {
	Name string
	// ExpiresAt is when the token stops working; the console defaults it to a year.
	ExpiresAt time.Time
}

type IngestTokenFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}
