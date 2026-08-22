package replay

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flight-observability/flight/internal/capture"
)

func TestExecutorRecordsBoundedSuccessfulReplay(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		responseWriter.Header().Set("Content-Type", "text/plain")
		_, _ = responseWriter.Write([]byte("long response"))
	}))
	defer target.Close()
	host, _, splitError := net.SplitHostPort(target.Listener.Addr().String())
	if splitError != nil {
		t.Fatal(splitError)
	}
	policy, policyError := NewPolicy([]string{host}, true)
	if policyError != nil {
		t.Fatal(policyError)
	}
	executor, executorError := NewExecutor(policy, time.Second, 4, 1)
	if executorError != nil {
		t.Fatal(executorError)
	}
	result := executor.Execute(context.Background(), capture.Request{Method: http.MethodGet, Target: target.URL})
	if result.Status != "succeeded" || result.HTTPStatus != http.StatusOK || !result.BodyTruncated || result.ResponseBytes != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecutorReturnsBlockedTarget(t *testing.T) {
	policy, policyError := NewPolicy([]string{"allowed.example.test"}, false)
	if policyError != nil {
		t.Fatal(policyError)
	}
	executor, executorError := NewExecutor(policy, time.Second, 1024, 1)
	if executorError != nil {
		t.Fatal(executorError)
	}
	result := executor.Execute(context.Background(), capture.Request{Method: http.MethodGet, Target: "https://blocked.example.test"})
	if result.Status != "blocked" || result.Reason == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
