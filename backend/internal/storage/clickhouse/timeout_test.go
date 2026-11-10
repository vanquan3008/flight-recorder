package clickhouse

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestResponseBodyReadPreservesTimeout(t *testing.T) {
	for _, source := range []string{"request context", "HTTP client"} {
		t.Run(source, func(t *testing.T) {
			store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			ctx := context.Background()
			if source == "HTTP client" {
				store.client.Timeout = 50 * time.Millisecond
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			if _, err := store.execute(ctx, "SELECT 1", nil, nil, false); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("response timeout lost its cause: %v", err)
			}
		})
	}
}
