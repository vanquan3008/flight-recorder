package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type retentionBackend struct {
	authOnlyBackend
	days    *int32
	project string
	updates int
}

func (backend *retentionBackend) GetProjectRetention(context.Context, string) (*int32, error) {
	return backend.days, nil
}

func (backend *retentionBackend) UpdateProjectRetention(_ context.Context, project string, days *int32, _ auth.Actor) error {
	backend.project, backend.days = project, days
	backend.updates++
	return nil
}

func TestProjectRetentionRequiresOwnerToSetOrInherit(t *testing.T) {
	backend := &retentionBackend{}
	server := newTestServer(nil)
	server.backend = backend
	server.defaultRetentionAge = 7 * 24 * time.Hour
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	path := "/api/v1/projects/" + testProjectIDForRoutes + "/retention"
	if res := call(router, http.MethodPut, path, `{"retention_days":3}`, "user"); res.Code != http.StatusForbidden {
		t.Fatalf("non-owner changed policy: %d %s", res.Code, res.Body)
	}
	for _, body := range []string{`{}`, `{"retention_days":0}`, `{"retention_days":106752}`, `{"retention_days":3.5}`} {
		if res := call(router, http.MethodPut, path, body, "owner"); res.Code != http.StatusBadRequest {
			t.Fatalf("invalid policy accepted: %s → %d %s", body, res.Code, res.Body)
		}
	}
	if backend.updates != 0 {
		t.Fatalf("invalid requests reached the store: %d", backend.updates)
	}
	if res := call(router, http.MethodPut, path, `{"retention_days":30}`, "owner"); res.Code != http.StatusOK {
		t.Fatalf("setting policy failed: %d %s", res.Code, res.Body)
	}
	if backend.project != testProjectIDForRoutes || backend.days == nil || *backend.days != 30 {
		t.Fatalf("wrong persisted policy: %+v", backend)
	}
	var policy projectRetentionResponse
	res := call(router, http.MethodGet, path, "", "user")
	if err := json.Unmarshal(res.Body.Bytes(), &policy); res.Code != http.StatusOK || err != nil || policy.Days == nil || *policy.Days != 30 || policy.DefaultAgeSeconds != 604800 {
		t.Fatalf("wrong policy shown to project member: %d %s", res.Code, res.Body)
	}
	if res := call(router, http.MethodPut, path, `{"retention_days":null}`, "owner"); res.Code != http.StatusOK || backend.days != nil {
		t.Fatalf("restoring the default failed: %d %s", res.Code, res.Body)
	}
}
