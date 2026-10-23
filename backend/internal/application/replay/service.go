package replay

import (
	"context"
	"errors"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay/capture"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

var ErrIncidentNotFound = errors.New("incident not found")

type ExecutionStore interface {
	StartReplayExecution(context.Context, string, string, string, string, string, string) (bool, error)
	CompleteReplayExecution(context.Context, string, string, replaydomain.Result) error
}

type RequestExecutor interface {
	Execute(context.Context, capture.Request) replaydomain.Result
}

type Command struct {
	ProjectID     string
	EnvironmentID string
	IncidentID    string
	ActorKeyID    string
	RequestID     string
	Request       capture.Request
}

type Outcome struct {
	ID     string              `json:"id"`
	Result replaydomain.Result `json:"result"`
}

type Service struct {
	store    ExecutionStore
	executor RequestExecutor
}

func NewService(store ExecutionStore, executor RequestExecutor) *Service {
	return &Service{store: store, executor: executor}
}

func (service *Service) Run(ctx context.Context, command Command) (Outcome, error) {
	executionID := telemetry.DeterministicID(time.Now(), command.ActorKeyID, command.IncidentID, command.RequestID)
	started, startError := service.store.StartReplayExecution(ctx, executionID, command.ProjectID, command.EnvironmentID, command.IncidentID, AuditTarget(command.Request.Target), command.ActorKeyID)
	if startError != nil {
		return Outcome{}, startError
	}
	if !started {
		return Outcome{}, ErrIncidentNotFound
	}
	result := service.executor.Execute(ctx, command.Request)
	completionContext, cancelCompletion := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCompletion()
	if completeError := service.store.CompleteReplayExecution(completionContext, executionID, command.ProjectID, result); completeError != nil {
		return Outcome{}, completeError
	}
	return Outcome{ID: executionID, Result: result}, nil
}
