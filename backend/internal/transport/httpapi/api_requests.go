package httpapi

import (
	"encoding/json"
	"time"
)

// Request bodies. Handlers decode into these types with unknown fields rejected,
// and the OpenAPI document publishes their schemas, so the two cannot drift.

type registerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type createProjectRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

type renameProjectRequest struct {
	Name string `json:"name"`
}

type createEnvironmentRequest struct {
	Name string `json:"name"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type changeMemberRoleRequest struct {
	Role string `json:"role"`
}

type createIngestTokenRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type createUserRequest struct {
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	Password      string `json:"password"`
	PlatformAdmin bool   `json:"platform_admin"`
}

type updateUserRequest struct {
	DisplayName   *string `json:"display_name"`
	PlatformAdmin *bool   `json:"platform_admin"`
	Disabled      *bool   `json:"disabled"`
}

type updateIncidentRequest struct {
	Status string `json:"status"`
}

type replayRequest struct {
	Target  string            `json:"target"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}
