package replay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay/capture"
	replaydomain "github.com/vanquan3008/tracepilot/internal/domain/replay"
)

type Executor struct {
	policy           *Policy
	client           *http.Client
	maxResponseBytes int64
	slots            chan struct{}
}

func NewExecutor(policy *Policy, timeout time.Duration, maxResponseBytes int64, maxConcurrent int) (*Executor, error) {
	if policy == nil || timeout <= 0 || maxResponseBytes <= 0 || maxConcurrent <= 0 {
		return nil, errors.New("replay executor configuration is invalid")
	}
	return &Executor{policy: policy, client: policy.HTTPClient(timeout), maxResponseBytes: maxResponseBytes, slots: make(chan struct{}, maxConcurrent)}, nil
}

func (executor *Executor) Execute(ctx context.Context, captured capture.Request) replaydomain.Result {
	startedAt := time.Now()
	result := replaydomain.Result{Target: AuditTarget(captured.Target)}
	target, validateError := executor.policy.Validate(ctx, captured.Target)
	if validateError != nil {
		result.Status, result.Reason, result.Duration = "blocked", validateError.Error(), time.Since(startedAt)
		return result
	}
	select {
	case executor.slots <- struct{}{}:
		defer func() { <-executor.slots }()
	case <-ctx.Done():
		result.Status, result.Reason, result.Duration = "blocked", "replay queue is full", time.Since(startedAt)
		return result
	}
	request, requestError := http.NewRequestWithContext(ctx, captured.Method, target.String(), bytes.NewReader(captured.Body))
	if requestError != nil {
		result.Status, result.Reason, result.Duration = "failed", "replay request could not be created", time.Since(startedAt)
		return result
	}
	request.Header = captured.Header.Clone()
	response, executeError := executor.client.Do(request)
	if executeError != nil {
		result.Status, result.Reason, result.Duration = "failed", "replay target could not be reached", time.Since(startedAt)
		return result
	}
	defer response.Body.Close()
	read, readError := io.CopyN(io.Discard, response.Body, executor.maxResponseBytes+1)
	result.Target = AuditTarget(target.String())
	result.Status = "succeeded"
	result.HTTPStatus = response.StatusCode
	result.Duration = time.Since(startedAt)
	result.ContentType = response.Header.Get("Content-Type")
	result.ResponseBytes = read
	result.BodyTruncated = read > executor.maxResponseBytes
	if result.BodyTruncated {
		result.ResponseBytes = executor.maxResponseBytes
	}
	if readError != nil && !errors.Is(readError, io.EOF) {
		result.Status, result.Reason = "failed", "replay response could not be read"
	}
	return result
}

func AuditTarget(rawTarget string) string {
	target, parseError := url.Parse(rawTarget)
	if parseError != nil || target.Scheme == "" || target.Host == "" {
		return "invalid-target"
	}
	target.User = nil
	target.RawQuery = ""
	target.ForceQuery = false
	target.Fragment = ""
	return target.String()
}
