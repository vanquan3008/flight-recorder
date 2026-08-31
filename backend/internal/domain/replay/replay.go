package replay

import (
	"encoding/json"
	"time"
)

type Result struct {
	Target        string        `json:"target"`
	Status        string        `json:"status"`
	Reason        string        `json:"reason,omitempty"`
	HTTPStatus    int           `json:"http_status,omitempty"`
	Duration      time.Duration `json:"duration"`
	ContentType   string        `json:"content_type,omitempty"`
	ResponseBytes int64         `json:"response_bytes"`
	BodyTruncated bool          `json:"body_truncated"`
}

type Execution struct {
	ID          string          `json:"id"`
	IncidentID  string          `json:"incident_id"`
	Target      string          `json:"target"`
	Status      string          `json:"status"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	DurationNS  *int64          `json:"duration_ns,omitempty"`
	HTTPStatus  *int            `json:"http_status,omitempty"`
	Comparison  json.RawMessage `json:"comparison,omitempty"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
}

type ExecutionFilter struct {
	Before   time.Time
	CursorID string
	Limit    int
}
