package incidents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

var principal = auth.Principal{UserID: "user", ProjectID: "project", EnvironmentID: "environment"}

type runner struct{}

func (runner) Run(context.Context, replay.Command) (replay.Outcome, error) {
	return replay.Outcome{}, nil
}

func TestParseFilter(t *testing.T) {
	from := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	filter, _, parseError := parseFilter(httptest.NewRequest(http.MethodGet, "/api/v1/incidents?service=checkout&severity=critical&status=resolved&from="+from.Format(time.RFC3339), nil))
	if parseError != nil {
		t.Fatal(parseError)
	}
	if filter.Service != "checkout" || filter.Severity != "CRITICAL" || filter.Status != "RESOLVED" || !filter.From.Equal(from) {
		t.Fatalf("filter = %#v", filter)
	}
	cursor := render.EncodeCursor(render.PageCursor{At: from, ID: "not-an-id"})
	for _, path := range []string{"/api/v1/incidents?status=closed", "/api/v1/incidents?cursor=" + cursor} {
		if _, _, parseError := parseFilter(httptest.NewRequest(http.MethodGet, path, nil)); parseError == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestReplayRouteExistsOnlyWithARunner(t *testing.T) {
	hasReplay := func(handler *Handler) bool {
		for _, route := range handler.Routes() {
			if route.OperationID == "replayIncident" {
				return true
			}
		}
		return false
	}
	if hasReplay(New(nil, nil, nil, 0, nil)) {
		t.Fatal("replay route mounted without a runner")
	}
	if !hasReplay(New(nil, nil, runner{}, 0, nil)) {
		t.Fatal("replay route missing with a runner")
	}
}

func TestIncidentAnalysisWithoutABackendIsNotImplemented(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/01900000-0000-7000-8000-000000000001/analysis", nil)
	request.SetPathValue("incidentID", "01900000-0000-7000-8000-000000000001")
	recorder := httptest.NewRecorder()
	New(nil, nil, nil, 0, nil).getAnalysis(recorder, request, principal)
	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d", recorder.Code)
	}
}
