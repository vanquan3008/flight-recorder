package pipeline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func TestStoreFailureCountsOnlyActualRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry := metrics.New("test", "test", "test")
		calls := 0
		pipeline := &Pipeline{batchTimeout: time.Second, metrics: registry, store: func(context.Context, []telemetry.Event) error {
			calls++
			return errors.New("storage unavailable")
		}}
		if err := pipeline.storeWithRetry(context.Background(), []item{{}}); err == nil || calls != 3 {
			t.Fatalf("store calls=%d error=%v, want three failed attempts", calls, err)
		}
		response := httptest.NewRecorder()
		registry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if !strings.Contains(response.Body.String(), "\ntracepilot_ingestion_retries_total 2\n") {
			t.Fatal("three attempts must record two retries")
		}
	})
}

func TestStoreRetryStopsDuringCancelledBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		pipeline := &Pipeline{batchTimeout: time.Second, store: func(context.Context, []telemetry.Event) error {
			calls++
			return errors.New("storage unavailable")
		}}
		done := make(chan error, 1)
		go func() { done <- pipeline.storeWithRetry(ctx, []item{{}}) }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("store calls=%d error=%v, want one attempt and cancellation", calls, err)
		}
	})
}
