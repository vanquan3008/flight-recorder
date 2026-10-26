// Package detection coordinates the baseline detectors and incident
// persistence. Detectors and storage are consumer-owned interfaces so the
// workflow is testable without a database.
package detection

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

// Detector reports evidence for the window ending at end.
type Detector interface {
	Detect(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error)
}

// DetectorFunc adapts a function to Detector.
type DetectorFunc func(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error)

func (detect DetectorFunc) Detect(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error) {
	return detect(ctx, end)
}

// IncidentStore persists one service's evidence atomically with its incident.
type IncidentStore interface {
	PersistObservabilityIncident(ctx context.Context, service string, end time.Time, evidence []analysis.Evidence) error
}

type Service struct {
	detectors []Detector
	store     IncidentStore
}

func NewService(store IncidentStore, detectors ...Detector) *Service {
	return &Service{detectors: detectors, store: store}
}

// Run evaluates every detector for the minute-aligned window ending at now and
// persists one incident per affected service. Any detector failure aborts the
// run before anything is written, so a partial view never opens incidents. The
// returned count is the number of services persisted so far.
func (service *Service) Run(ctx context.Context, now time.Time) (int, error) {
	end := now.UTC().Truncate(time.Minute)
	byService := make(map[string][]analysis.Evidence)
	for _, detector := range service.detectors {
		found, err := detector.Detect(ctx, end)
		if err != nil {
			return 0, err
		}
		for _, detected := range found {
			byService[detected.Service] = append(byService[detected.Service], detected.Item)
		}
	}
	services := make([]string, 0, len(byService))
	for name, evidence := range byService {
		if len(evidence) > 0 {
			services = append(services, name)
		}
	}
	sort.Strings(services)
	count := 0
	for _, name := range services {
		if err := service.store.PersistObservabilityIncident(ctx, name, end, byService[name]); err != nil {
			return count, fmt.Errorf("persist observability incident for %s: %w", name, err)
		}
		count++
	}
	return count, nil
}
