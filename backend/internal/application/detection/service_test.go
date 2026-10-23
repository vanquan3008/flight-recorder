package detection

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

type recordingStore struct {
	services []string
	evidence map[string][]string
	ends     []time.Time
	failOn   string
}

func (store *recordingStore) PersistObservabilityIncident(_ context.Context, service string, end time.Time, evidence []analysis.Evidence) error {
	if service == store.failOn {
		return errors.New("storage unavailable")
	}
	if store.evidence == nil {
		store.evidence = map[string][]string{}
	}
	store.services = append(store.services, service)
	store.ends = append(store.ends, end)
	for _, item := range evidence {
		store.evidence[service] = append(store.evidence[service], item.MetricName)
	}
	return nil
}

func finding(service, metric string) analysis.DetectedEvidence {
	return analysis.DetectedEvidence{Service: service, Item: analysis.Evidence{MetricName: metric}}
}

func fixed(found ...analysis.DetectedEvidence) Detector {
	return DetectorFunc(func(context.Context, time.Time) ([]analysis.DetectedEvidence, error) { return found, nil })
}

func TestRunGroupsEvidenceByServiceAcrossDetectorsInStableOrder(t *testing.T) {
	store := &recordingStore{}
	service := NewService(store,
		fixed(finding("payments", "error_rate"), finding("checkout", "latency")),
		fixed(finding("checkout", "dependency")),
		fixed(),
	)
	count, err := service.Run(context.Background(), time.Date(2026, 10, 14, 10, 7, 41, 0, time.FixedZone("x", 3600)))
	if err != nil || count != 2 {
		t.Fatalf("Run = %d, %v", count, err)
	}
	if !reflect.DeepEqual(store.services, []string{"checkout", "payments"}) {
		t.Fatalf("services = %v", store.services)
	}
	if !reflect.DeepEqual(store.evidence["checkout"], []string{"latency", "dependency"}) {
		t.Fatalf("checkout evidence = %v", store.evidence["checkout"])
	}
	want := time.Date(2026, 10, 14, 9, 7, 0, 0, time.UTC)
	if !store.ends[0].Equal(want) {
		t.Fatalf("window end = %v, want minute-aligned UTC %v", store.ends[0], want)
	}
}

func TestRunWritesNothingWhenAnyDetectorFails(t *testing.T) {
	store := &recordingStore{}
	failing := DetectorFunc(func(context.Context, time.Time) ([]analysis.DetectedEvidence, error) {
		return nil, errors.New("query timeout")
	})
	count, err := NewService(store, fixed(finding("checkout", "latency")), failing).Run(context.Background(), time.Now())
	if err == nil || count != 0 || len(store.services) != 0 {
		t.Fatalf("Run = %d, %v; persisted %v", count, err, store.services)
	}
}

func TestRunReportsPartialProgressAndNamesTheFailingService(t *testing.T) {
	store := &recordingStore{failOn: "payments"}
	count, err := NewService(store, fixed(finding("checkout", "a"), finding("payments", "b"), finding("search", "c"))).Run(context.Background(), time.Now())
	if err == nil || count != 1 {
		t.Fatalf("Run = %d, %v; want one persisted before the failure", count, err)
	}
	if !strings.Contains(err.Error(), "payments") || len(store.services) != 1 {
		t.Fatalf("error should name the failing service, got %v (persisted %v)", err, store.services)
	}
}

func TestRunWithoutEvidenceIsQuiet(t *testing.T) {
	store := &recordingStore{}
	count, err := NewService(store, fixed()).Run(context.Background(), time.Now())
	if err != nil || count != 0 || len(store.services) != 0 {
		t.Fatalf("Run = %d, %v", count, err)
	}
}
