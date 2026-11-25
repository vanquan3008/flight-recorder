package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrEmailExists         = errors.New("email already registered")
	ErrUserDisabled        = errors.New("user is disabled")
	ErrProjectNotFound     = errors.New("project not found")
	ErrProjectSlugExists   = errors.New("project slug already exists")
	ErrEnvironmentExists   = errors.New("environment already exists")
	ErrMemberNotFound      = errors.New("member not found")
	ErrMemberExists        = errors.New("user is already a member")
	ErrLastOwner           = errors.New("a project must keep at least one owner")
	ErrLastPlatformAdmin   = errors.New("the super administrator cannot be disabled")
	ErrPlatformAdminExists = errors.New("a super administrator already exists")
	ErrTooManyProjects     = errors.New("project limit reached")
	ErrIngestTokenNotFound = errors.New("ingest token not found")
	ErrIngestTokenExists   = errors.New("ingest token name already exists")
	ErrInvalidIdentity     = errors.New("invalid identity input")
	ErrSessionNotFound     = errors.New("session not found")
	ErrForbiddenRoleChange = errors.New("role change not permitted")
)

var (
	emailPattern       = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
	projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// User is an account. PasswordHash never leaves the storage layer in a response.
type User struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	PlatformAdmin      bool       `json:"platform_admin"`
	MustChangePassword bool       `json:"must_change_password"`
	DisabledAt         *time.Time `json:"disabled_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	// TokensValidAfter is a monotonically increasing revocation marker, never sent to clients.
	TokensValidAfter time.Time `json:"-"`
}

// TokenVersion uses PostgreSQL's timestamp precision, independently of JWT iat and server clocks.
func (user User) TokenVersion() int64 {
	if user.TokensValidAfter.IsZero() {
		return 0
	}
	return user.TokensValidAfter.UnixMicro()
}

// Credentials is a user with the secret needed to verify a login.
type Credentials struct {
	User         User
	PasswordHash string
}

type Project struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Environment struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Scope is one environment of one project: the unit telemetry is stored and analysed in.
type Scope struct {
	Project     Project
	Environment Environment
}

// Membership is one project a user can open, with the role they hold there.
type Membership struct {
	Project Project `json:"project"`
	Role    string  `json:"role"`
}

type Member struct {
	User      User      `json:"user"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Session is a validated refresh session. The token itself is never stored.
type Session struct {
	UserID    string
	ExpiresAt time.Time
}

type AuditEvent struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id,omitempty"`
	ActorUserID string         `json:"actor_user_id,omitempty"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	Detail      map[string]any `json:"detail"`
	CreatedAt   time.Time      `json:"created_at"`
}

type CreateUserParams struct {
	Email         string
	DisplayName   string
	Password      string
	PlatformAdmin bool
	// MustChangePassword marks a password an administrator chose or generated.
	MustChangePassword bool
}

type CreateProjectParams struct {
	Slug string
	Name string
	// EnvironmentName names the first environment; it defaults to "production".
	EnvironmentName string
}

// UpdateUserParams changes only the fields that are set.
type UpdateUserParams struct {
	DisplayName *string
	Disabled    *bool
}

type UserFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

type AuditFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}

// NormalizeEmail lowercases and validates an address; it does not verify delivery.
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 || !emailPattern.MatchString(email) {
		return "", fmt.Errorf("%w: email is not valid", ErrInvalidIdentity)
	}
	return email, nil
}

func NormalizeDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return "", fmt.Errorf("%w: display name must be 1-100 characters", ErrInvalidIdentity)
	}
	return name, nil
}

func NormalizeCreateUserParams(params CreateUserParams) (CreateUserParams, error) {
	var validationError error
	if params.Email, validationError = NormalizeEmail(params.Email); validationError != nil {
		return CreateUserParams{}, validationError
	}
	if params.DisplayName, validationError = NormalizeDisplayName(params.DisplayName); validationError != nil {
		return CreateUserParams{}, validationError
	}
	if validationError = ValidatePassword(params.Password); validationError != nil {
		return CreateUserParams{}, fmt.Errorf("%w: %v", ErrInvalidIdentity, validationError)
	}
	return params, nil
}

func NormalizeCreateProjectParams(params CreateProjectParams) (CreateProjectParams, error) {
	params.Slug = strings.ToLower(strings.TrimSpace(params.Slug))
	if !projectSlugPattern.MatchString(params.Slug) {
		return CreateProjectParams{}, fmt.Errorf("%w: slug must be 1-63 lowercase letters, digits or hyphens", ErrInvalidIdentity)
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" || len([]rune(params.Name)) > 100 {
		return CreateProjectParams{}, fmt.Errorf("%w: project name must be 1-100 characters", ErrInvalidIdentity)
	}
	params.EnvironmentName = strings.TrimSpace(params.EnvironmentName)
	if params.EnvironmentName == "" {
		params.EnvironmentName = "production"
	}
	if len([]rune(params.EnvironmentName)) > 63 {
		return CreateProjectParams{}, fmt.Errorf("%w: environment name must be at most 63 characters", ErrInvalidIdentity)
	}
	return params, nil
}

// NormalizeEnvironmentName trims and bounds an environment name.
func NormalizeEnvironmentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 63 {
		return "", fmt.Errorf("%w: environment name must be 1-63 characters", ErrInvalidIdentity)
	}
	return name, nil
}

const (
	maxIngestTokenLifetime = 3 * 365 * 24 * time.Hour
	// MaxOwnedProjects bounds how many projects an ordinary account may own; super administrators are not limited.
	MaxOwnedProjects = 10
)

var ingestTokenNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

// NormalizeCreateIngestTokenParams checks the name and that the expiry is in the future and not
// beyond three years; an unset expiry means one year.
func NormalizeCreateIngestTokenParams(params CreateIngestTokenParams, now time.Time) (CreateIngestTokenParams, error) {
	params.Name = strings.TrimSpace(params.Name)
	if !ingestTokenNamePattern.MatchString(params.Name) {
		return CreateIngestTokenParams{}, fmt.Errorf("%w: name must be 1-64 letters, digits, spaces, dots, underscores, or hyphens", ErrInvalidIdentity)
	}
	if params.ExpiresAt.IsZero() {
		params.ExpiresAt = now.Add(365 * 24 * time.Hour)
	}
	params.ExpiresAt = params.ExpiresAt.UTC()
	if !params.ExpiresAt.After(now) || params.ExpiresAt.After(now.Add(maxIngestTokenLifetime)) {
		return CreateIngestTokenParams{}, fmt.Errorf("%w: expiry must be in the future and within three years", ErrInvalidIdentity)
	}
	return params, nil
}
