// Package notification delivers durable, signed webhook notifications.
package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	notificationdomain "github.com/vanquan3008/tracepilot/internal/domain/notification"
)

type Store interface {
	RecordDeliveryAttempt(context.Context, string, string, string, int, string, bool) error
}

type RetryStore interface {
	ClaimDueDeliveries(context.Context, int) ([]notificationdomain.DeliveryAttempt, error)
}

// Observer receives bounded-cardinality delivery outcomes without coupling
// the notification package to a concrete metrics implementation.
type Observer interface {
	ObserveWebhookAttempt(string)
}

type Config struct {
	PollInterval time.Duration
	Workers      int
	Timeout      time.Duration
	MaxResponse  int64
	Observer     Observer
}

type Service struct {
	store    Store
	client   *http.Client
	logger   *slog.Logger
	config   Config
	observer Observer
	request  sync.WaitGroup
}

func NewService(store Store, logger *slog.Logger, configurations ...Config) *Service {
	configuration := Config{PollInterval: 5 * time.Second, Workers: 4, Timeout: 10 * time.Second, MaxResponse: 64 << 10}
	if len(configurations) > 0 {
		configuration.Observer = configurations[0].Observer
		if configurations[0].PollInterval > 0 {
			configuration.PollInterval = configurations[0].PollInterval
		}
		if configurations[0].Workers > 0 {
			configuration.Workers = configurations[0].Workers
		}
		if configurations[0].Timeout > 0 {
			configuration.Timeout = configurations[0].Timeout
		}
		if configurations[0].MaxResponse > 0 {
			configuration.MaxResponse = configurations[0].MaxResponse
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, logger: logger, config: configuration, observer: configuration.Observer, client: safeClient(configuration.Timeout)}
}

func (service *Service) Run(ctx context.Context) error {
	store, ok := service.store.(RetryStore)
	if !ok {
		return errors.New("notification store does not implement retry worker")
	}
	service.processDue(ctx, store)
	ticker := time.NewTicker(service.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			service.request.Wait()
			return nil
		case <-ticker.C:
			service.processDue(ctx, store)
		}
	}
}

func (service *Service) processDue(ctx context.Context, store RetryStore) {
	batch, claimError := store.ClaimDueDeliveries(ctx, service.config.Workers)
	if claimError != nil {
		service.observe("claim_error")
		service.logger.Error("claim webhook deliveries failed", "error", claimError)
		return
	}
	semaphore := make(chan struct{}, service.config.Workers)
	for _, attempt := range batch {
		service.request.Add(1)
		go func(attempt notificationdomain.DeliveryAttempt) {
			defer service.request.Done()
			semaphore <- struct{}{}
			service.dispatchAttempt(ctx, attempt)
			<-semaphore
		}(attempt)
	}
	service.request.Wait()
}

func (service *Service) dispatchAttempt(ctx context.Context, attempt notificationdomain.DeliveryAttempt) {
	service.dispatch(ctx, attempt.ID, notificationdomain.WebhookRecord{ID: attempt.WebhookID, ProjectID: attempt.ProjectID, URL: attempt.URL, Secret: attempt.Secret}, attempt.Payload, attempt.ProjectID)
}

func (service *Service) dispatch(ctx context.Context, deliveryID string, hook notificationdomain.WebhookRecord, payload []byte, projectID string) {
	signature := sign(payload, hook.Secret)
	request, requestError := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(payload))
	if requestError != nil {
		service.recordFailure(ctx, deliveryID, hook, projectID, 0, requestError)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "TracePilot-Webhook/1")
	request.Header.Set("X-TracePilot-Signature", "sha256="+signature)
	request.Header.Set("X-TracePilot-Delivery", deliveryID)
	response, doError := service.client.Do(request)
	if doError != nil {
		service.recordFailure(ctx, deliveryID, hook, projectID, 0, doError)
		return
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, service.config.MaxResponse+1)
	success := response.StatusCode >= 200 && response.StatusCode < 300
	if success {
		service.observe("success")
		if recordError := service.store.RecordDeliveryAttempt(ctx, deliveryID, hook.ID, projectID, response.StatusCode, "", true); recordError != nil {
			service.logger.Error("record webhook success failed", "delivery_id", deliveryID, "error", recordError)
		}
		return
	}
	service.recordFailure(ctx, deliveryID, hook, projectID, response.StatusCode, fmt.Errorf("webhook returned HTTP %d", response.StatusCode))
}

func (service *Service) recordFailure(ctx context.Context, deliveryID string, hook notificationdomain.WebhookRecord, projectID string, status int, deliveryError error) {
	service.observe("failure")
	if recordError := service.store.RecordDeliveryAttempt(ctx, deliveryID, hook.ID, projectID, status, deliveryError.Error(), false); recordError != nil {
		service.logger.Error("record webhook failure failed", "delivery_id", deliveryID, "error", recordError)
	}
	service.logger.Warn("webhook delivery failed", "webhook_id", hook.ID, "status", status, "error", deliveryError)
}

func (service *Service) observe(outcome string) {
	if service.observer != nil {
		service.observer.ObserveWebhookAttempt(outcome)
	}
}

func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
