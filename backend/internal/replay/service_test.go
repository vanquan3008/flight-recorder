package replay

import (
	"context"
	"errors"
	"testing"

	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/replay/capture"
)

type serviceStore struct {
	started   bool
	completed replaydomain.Result
}

func (store *serviceStore) StartReplayExecution(context.Context, string, string, string, string, string, string) (bool, error) {
	return store.started, nil
}

func (store *serviceStore) CompleteReplayExecution(_ context.Context, _, _ string, result replaydomain.Result) error {
	store.completed = result
	return nil
}

type serviceExecutor struct{ result replaydomain.Result }

func (executor serviceExecutor) Execute(context.Context, capture.Request) replaydomain.Result {
	return executor.result
}

func TestServiceRunsAndRecordsReplay(t *testing.T) {
	store := &serviceStore{started: true}
	want := replaydomain.Result{Status: "succeeded", HTTPStatus: 200}
	outcome, runError := NewService(store, serviceExecutor{result: want}).Run(context.Background(), Command{
		ProjectID: "project", EnvironmentID: "environment", IncidentID: "incident", ActorKeyID: "key", RequestID: "request",
		Request: capture.Request{Target: "https://example.test/path?secret=value"},
	})
	if runError != nil || outcome.ID == "" || outcome.Result.Status != want.Status || store.completed.Status != want.Status {
		t.Fatalf("outcome=%+v completed=%+v error=%v", outcome, store.completed, runError)
	}
}

func TestServiceRejectsUnknownIncident(t *testing.T) {
	_, runError := NewService(&serviceStore{}, serviceExecutor{}).Run(context.Background(), Command{})
	if !errors.Is(runError, ErrIncidentNotFound) {
		t.Fatalf("error=%v", runError)
	}
}
