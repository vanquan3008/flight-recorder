package retention

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

type policyStore interface {
	GetProjectRetention(context.Context, string) (*int32, error)
	UpdateProjectRetention(context.Context, string, *int32, auth.Actor) error
}

// PolicyService keeps query cutoffs and policy updates on the same local cache.
type PolicyService struct {
	store    policyStore
	cache    *retentionCache
	fallback time.Duration
	now      func() time.Time
}

func NewPolicyService(store policyStore, fallback, ttl time.Duration, limit int, now func() time.Time) *PolicyService {
	return &PolicyService{store: store, cache: newRetentionCache(ttl, limit), fallback: fallback, now: now}
}

// GetProjectRetention reads the persisted value for the settings API.
func (service *PolicyService) GetProjectRetention(ctx context.Context, project string) (*int32, error) {
	return service.store.GetProjectRetention(ctx, project)
}

func (service *PolicyService) UpdateProjectRetention(ctx context.Context, project string, days *int32, actor auth.Actor) error {
	if err := service.store.UpdateProjectRetention(ctx, project, days, actor); err != nil {
		return err
	}
	service.cache.forget(project)
	return nil
}

func (service *PolicyService) Cutoff(ctx context.Context, project string) (time.Time, error) {
	now := service.now()
	days, err := service.cache.get(ctx, project, now, service.store.GetProjectRetention)
	if err != nil {
		return time.Time{}, err
	}
	if age := telemetry.RetentionAge(days, service.fallback); age > 0 {
		cutoff := now.UTC().Add(-age)
		if cutoff.After(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return cutoff, nil
		}
	}
	return time.Time{}, nil
}
