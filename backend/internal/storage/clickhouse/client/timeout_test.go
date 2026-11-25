package client_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"
)

func TestResponseBodyReadPreservesTimeout(t *testing.T) {
	for _, source := range []string{"request context", "HTTP client"} {
		t.Run(source, func(t *testing.T) {
			store := clienttest.Client(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			ctx := context.Background()
			if source == "HTTP client" {
				store.HTTP.Timeout = 50 * time.Millisecond
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			if _, err := store.Execute(ctx, "SELECT 1", nil, nil, false); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("response timeout lost its cause: %v", err)
			}
		})
	}
}
