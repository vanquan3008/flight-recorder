// Package incidentapp contains incident mutation use cases.
package incidentapp

import (
	"context"
	"errors"

	"github.com/vanquan3008/tracepilot/internal/domain/incident"
)

var ErrInvalidStatus = errors.New("invalid incident status")

// StatusStore persists an atomic incident status transition.
type StatusStore interface {
	UpdateIncidentStatus(context.Context, string, string, string, string, string) (incident.Incident, error)
}

// UpdateStatusCommand identifies the incident and the authenticated actor
// requesting its next status.
type UpdateStatusCommand struct {
	ProjectID     string
	EnvironmentID string
	IncidentID    string
	Status        string
	ActorKeyID    string
}

// Service owns incident command validation before delegating the atomic state
// transition to its persistence port.
type Service struct {
	store StatusStore
}

func New(store StatusStore) *Service {
	return &Service{store: store}
}

func (service *Service) UpdateStatus(ctx context.Context, command UpdateStatusCommand) (incident.Incident, error) {
	status, valid := incident.NormalizeStatus(command.Status)
	if !valid {
		return incident.Incident{}, ErrInvalidStatus
	}
	if command.ActorKeyID == "" {
		return incident.Incident{}, incident.ErrInvalidTransition
	}
	return service.store.UpdateIncidentStatus(ctx, command.ProjectID, command.EnvironmentID, command.IncidentID, status, command.ActorKeyID)
}
