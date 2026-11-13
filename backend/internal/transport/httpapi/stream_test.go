package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/live"
	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestStreamHandlerStopsWhenHubCloses(t *testing.T) {
	hub := live.NewHub()
	server := &Server{streamHub: hub}
	ctx := context.Background()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil).WithContext(ctx)
	recorder := newStreamRecorder()
	done := make(chan struct{})
	go func() {
		server.streamLiveTelemetry(recorder, request, auth.Principal{ProjectID: "project", EnvironmentID: "environment"})
		close(done)
	}()

	waitForStreamFrame(t, recorder)
	hub.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not stop when the hub closed")
	}
}

type deadlineResponseRecorder struct {
	*httptest.ResponseRecorder
	frames chan struct{}
}

func (*deadlineResponseRecorder) SetWriteDeadline(time.Time) error { return nil }

func newStreamRecorder() *deadlineResponseRecorder {
	return &deadlineResponseRecorder{ResponseRecorder: httptest.NewRecorder(), frames: make(chan struct{}, 4)}
}

func (recorder *deadlineResponseRecorder) Flush() {
	recorder.ResponseRecorder.Flush()
	recorder.frames <- struct{}{}
}

func waitForStreamFrame(t *testing.T, recorder *deadlineResponseRecorder) {
	t.Helper()
	select {
	case <-recorder.frames:
	case <-time.After(time.Second):
		t.Fatal("stream did not flush its frame")
	}
}

func TestStreamLiveTelemetryHandler(t *testing.T) {
	hub := live.NewHub()
	server := &Server{
		streamHub: hub,
	}

	projectID := "proj-test"
	envID := "env-test"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/live/stream", nil).WithContext(ctx)
	recorder := newStreamRecorder()

	done := make(chan struct{})
	go func() {
		server.streamLiveTelemetry(recorder, request, auth.Principal{
			ProjectID:     projectID,
			EnvironmentID: envID,
		})
		close(done)
	}()

	waitForStreamFrame(t, recorder)

	hub.Broadcast(live.Event{
		Type:          "log",
		ProjectID:     projectID,
		EnvironmentID: envID,
		Payload:       "test-payload",
	})

	waitForStreamFrame(t, recorder)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("stream handler did not terminate after cancel")
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", contentType)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: connected\ndata: {}\n\n") || !strings.Contains(body, `"type":"log","project_id":"proj-test","environment_id":"env-test","payload":"test-payload"`) {
		t.Fatalf("stream response contract changed: %s", body)
	}
}
