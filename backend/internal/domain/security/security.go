// Package security contains the vendor-neutral security analytics model.
package security

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

var (
	ErrInvalidRule = errors.New("invalid detection rule")
)

// Event is the normalized, searchable view of an immutable raw log record.
// RawEventID always points back to the original telemetry event.
type Event struct {
	ID               string         `json:"id"`
	RawEventID       string         `json:"raw_event_id"`
	SourceID         string         `json:"source_id,omitempty"`
	SourceName       string         `json:"source_name,omitempty"`
	OccurredAt       time.Time      `json:"occurred_at"`
	Format           string         `json:"format"`
	Kind             string         `json:"kind"`
	Category         string         `json:"category,omitempty"`
	Action           string         `json:"action,omitempty"`
	Outcome          string         `json:"outcome,omitempty"`
	Severity         string         `json:"severity,omitempty"`
	ObserverVendor   string         `json:"observer_vendor,omitempty"`
	ObserverProduct  string         `json:"observer_product,omitempty"`
	ObserverHostname string         `json:"observer_hostname,omitempty"`
	HostName         string         `json:"host_name,omitempty"`
	UserName         string         `json:"user_name,omitempty"`
	SourceIP         string         `json:"source_ip,omitempty"`
	SourcePort       int            `json:"source_port,omitempty"`
	DestinationIP    string         `json:"destination_ip,omitempty"`
	DestinationPort  int            `json:"destination_port,omitempty"`
	NetworkTransport string         `json:"network_transport,omitempty"`
	NetworkProtocol  string         `json:"network_protocol,omitempty"`
	RuleIdentifier   string         `json:"rule_identifier,omitempty"`
	RuleName         string         `json:"rule_name,omitempty"`
	Message          string         `json:"message,omitempty"`
	Attributes       map[string]any `json:"attributes,omitempty"`
}

type Source struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Format        string    `json:"format"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	AcceptedCount int64     `json:"accepted_count"`
	ErrorCount    int64     `json:"error_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type Filter struct {
	EnvironmentID string
	Kind          string
	Severity      string
	SourceIP      string
	DestinationIP string
	Outcome       string
	From          time.Time
	To            time.Time
	Before        time.Time
	CursorID      string
	Limit         int
}

// Rule deliberately supports one predicate. Combining arbitrary predicates is
// postponed until there is a concrete rule-pack requirement to keep both the
// data model and matching behavior auditable.
type Rule struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Version       int           `json:"version"`
	Enabled       bool          `json:"enabled"`
	Severity      string        `json:"severity"`
	Field         string        `json:"field"`
	Operator      string        `json:"operator"`
	Value         string        `json:"value,omitempty"`
	Threshold     int           `json:"threshold"`
	GroupBy       string        `json:"group_by,omitempty"`
	Window        time.Duration `json:"-"`
	WindowSeconds int           `json:"window_seconds"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type Detection struct {
	ID              string    `json:"id"`
	RuleID          string    `json:"rule_id"`
	RuleVersion     int       `json:"rule_version"`
	SecurityEventID string    `json:"security_event_id"`
	IncidentID      string    `json:"incident_id,omitempty"`
	GroupKey        string    `json:"group_key,omitempty"`
	WindowStartedAt time.Time `json:"window_started_at"`
	WindowEndedAt   time.Time `json:"window_ended_at"`
	Severity        string    `json:"severity"`
	CreatedAt       time.Time `json:"created_at"`
}

// RuleAudit is an immutable snapshot of a rule mutation. It remains available
// after a rule is disabled, so operators can explain historical detections.
type RuleAudit struct {
	ID         string         `json:"id"`
	RuleID     string         `json:"rule_id"`
	Version    int            `json:"version"`
	Action     string         `json:"action"`
	ActorKeyID string         `json:"actor_key_id,omitempty"`
	Rule       map[string]any `json:"rule"`
	CreatedAt  time.Time      `json:"created_at"`
}

// validRuleFields and validOperators use sets instead of switch statements so
// adding a new field or operator requires only one change here rather than
// updating every call site.
var validRuleFields = map[string]struct{}{
	"kind":             {},
	"category":         {},
	"action":           {},
	"outcome":          {},
	"severity":         {},
	"source_ip":        {},
	"destination_ip":   {},
	"observer_vendor":  {},
	"observer_product": {},
	"host_name":        {},
	"user_name":        {},
	"rule_identifier":  {},
}

var validOperators = map[string]struct{}{
	"equals":   {},
	"contains": {},
	"cidr":     {},
	"exists":   {},
}

func (event Event) Validate() error {
	if event.ID == "" || event.RawEventID == "" || event.OccurredAt.IsZero() || strings.TrimSpace(event.Kind) == "" {
		return errors.New("security event identity, timestamp, and kind are required")
	}
	for _, value := range []string{event.SourceIP, event.DestinationIP} {
		if value == "" {
			continue
		}
		if _, parseError := netip.ParseAddr(value); parseError != nil {
			return fmt.Errorf("security event IP is invalid: %w", parseError)
		}
	}
	if event.SourcePort < 0 || event.SourcePort > 65535 || event.DestinationPort < 0 || event.DestinationPort > 65535 {
		return errors.New("security event port is invalid")
	}
	return nil
}

func (rule Rule) Validate() error {
	if strings.TrimSpace(rule.Name) == "" {
		return ErrInvalidRule
	}
	if !validRuleField(rule.Field) {
		return ErrInvalidRule
	}
	if !validOperator(rule.Operator) {
		return ErrInvalidRule
	}
	if rule.Threshold < 1 || rule.Window <= 0 {
		return ErrInvalidRule
	}
	if rule.Operator != "exists" && strings.TrimSpace(rule.Value) == "" {
		return ErrInvalidRule
	}
	if rule.Operator == "cidr" {
		if _, parseError := netip.ParsePrefix(rule.Value); parseError != nil {
			return ErrInvalidRule
		}
	}
	if rule.GroupBy != "" && !validRuleField(rule.GroupBy) {
		return ErrInvalidRule
	}
	return nil
}

func (event Event) Field(name string) string {
	switch name {
	case "kind":
		return event.Kind
	case "category":
		return event.Category
	case "action":
		return event.Action
	case "outcome":
		return event.Outcome
	case "severity":
		return event.Severity
	case "source_ip":
		return event.SourceIP
	case "destination_ip":
		return event.DestinationIP
	case "observer_vendor":
		return event.ObserverVendor
	case "observer_product":
		return event.ObserverProduct
	case "host_name":
		return event.HostName
	case "user_name":
		return event.UserName
	case "rule_identifier":
		return event.RuleIdentifier
	default:
		return ""
	}
}

func Matches(rule Rule, event Event) bool {
	actual := event.Field(rule.Field)
	switch rule.Operator {
	case "exists":
		return actual != ""
	case "equals":
		return strings.EqualFold(actual, rule.Value)
	case "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(rule.Value))
	case "cidr":
		address, addressError := netip.ParseAddr(actual)
		prefix, prefixError := netip.ParsePrefix(rule.Value)
		return addressError == nil && prefixError == nil && prefix.Contains(address)
	default:
		return false
	}
}

func validRuleField(value string) bool {
	_, ok := validRuleFields[value]
	return ok
}

func validOperator(value string) bool {
	_, ok := validOperators[value]
	return ok
}
