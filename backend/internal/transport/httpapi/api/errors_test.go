package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/observability/logging"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render/rendertest"
)

func TestInternalErrorsLogTheRequestTraceAndHideTheCause(t *testing.T) {
	var logOutput bytes.Buffer
	errs := Errors{Logger: logging.NewLogger(&logOutput, nil)}
	traceContext := logging.ContinueTrace("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	request = request.WithContext(logging.WithTraceContext(request.Context(), traceContext))
	response := httptest.NewRecorder()

	errs.Internal(response, request, "list traces", errors.New("dial tcp 10.0.0.7:5432: connection refused"))

	if response.Code != http.StatusInternalServerError || rendertest.ErrorCode(t, response) != "INTERNAL_ERROR" {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "10.0.0.7") {
		t.Fatalf("the response revealed the cause: %s", response.Body)
	}
	for _, field := range []string{`"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"span_id":"00f067aa0ba902b7"`, `"msg":"list traces"`} {
		if !strings.Contains(logOutput.String(), field) {
			t.Fatalf("log line %s is missing %s", logOutput.String(), field)
		}
	}
}

func TestInternalErrorsReportTimeoutsAsQueryTimeouts(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	response := httptest.NewRecorder()

	Errors{}.Internal(response, request, "list logs", fmt.Errorf("query logs: %w", context.DeadlineExceeded))

	if response.Code != http.StatusGatewayTimeout || rendertest.ErrorCode(t, response) != "QUERY_TIMEOUT" {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
}

func TestIdentityErrorsMapToStableCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("create user: %w: email is invalid", auth.ErrInvalidIdentity), http.StatusBadRequest, "INVALID_REQUEST"},
		{auth.ErrEmailExists, http.StatusConflict, "EMAIL_EXISTS"},
		{auth.ErrLastOwner, http.StatusConflict, "LAST_OWNER"},
		{auth.ErrProjectNotFound, http.StatusNotFound, "NOT_FOUND"},
		{auth.ErrForbiddenRoleChange, http.StatusForbidden, "FORBIDDEN"},
		{errors.New("connection refused"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		response := httptest.NewRecorder()
		if !(Errors{}).Identity(response, httptest.NewRequest(http.MethodGet, "/", nil), "operation", tc.err) {
			t.Fatalf("%v: not answered", tc.err)
		}
		if response.Code != tc.status || rendertest.ErrorCode(t, response) != tc.code {
			t.Fatalf("%v: %d %s", tc.err, response.Code, response.Body)
		}
	}
	if (Errors{}).Identity(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), "operation", nil) {
		t.Fatal("answered a nil error")
	}
}

func TestInvalidIdentityKeepsOnlyTheValidationDetail(t *testing.T) {
	response := httptest.NewRecorder()
	(Errors{}).Identity(response, httptest.NewRequest(http.MethodGet, "/", nil), "create user", fmt.Errorf("create user: %w: email is invalid", auth.ErrInvalidIdentity))
	if !strings.Contains(response.Body.String(), `"message":"email is invalid"`) {
		t.Fatalf("body = %s", response.Body)
	}
}

func TestDecodeBodyRejectsMissingAndUnknownFields(t *testing.T) {
	var payload struct {
		Name string `json:"name"`
	}
	for body, message := range map[string]string{"": "request body is required", `{"other":1}`: "unknown field"} {
		response := httptest.NewRecorder()
		if DecodeBody(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), &payload) {
			t.Fatalf("%q decoded", body)
		}
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), message) {
			t.Fatalf("%q: %d %s", body, response.Code, response.Body)
		}
	}
}

func TestRequireProjectAndVisibleProjects(t *testing.T) {
	response := httptest.NewRecorder()
	if RequireProject(response, auth.Principal{}) || response.Code != http.StatusForbidden {
		t.Fatalf("no project: %d", response.Code)
	}
	if !RequireProject(httptest.NewRecorder(), auth.Principal{ProjectID: "project"}) {
		t.Fatal("rejected a selected project")
	}
	lister := memberships{}
	if all, _ := VisibleProjects(context.Background(), lister, auth.User{ID: "admin", PlatformAdmin: true}); len(all) != 2 {
		t.Fatalf("super administrator sees %d projects", len(all))
	}
	if own, _ := VisibleProjects(context.Background(), lister, auth.User{ID: "user"}); len(own) != 1 || own[0].Project.ID != "user" {
		t.Fatalf("account sees %+v", own)
	}
}

type memberships struct{}

func (memberships) ListAllProjects(context.Context) ([]auth.Membership, error) {
	return []auth.Membership{{Project: auth.Project{ID: "a"}}, {Project: auth.Project{ID: "b"}}}, nil
}

func (memberships) ListMemberships(_ context.Context, userID string) ([]auth.Membership, error) {
	return []auth.Membership{{Project: auth.Project{ID: userID}}}, nil
}

func TestBearerTokenReadsOnlyTheBearerScheme(t *testing.T) {
	for header, want := range map[string]string{"Bearer abc": "abc", "bearer  abc ": "abc", "Basic abc": "", "Bearer": "", "": ""} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", header)
		if got := BearerToken(request); got != want {
			t.Fatalf("%q: token = %q, want %q", header, got, want)
		}
	}
}
