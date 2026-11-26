package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type testPolicyStore struct {
	retentionLoader
	updateError error
}

func (store *testPolicyStore) GetProjectRetention(ctx context.Context, project string) (*int32, error) {
	return store.load(ctx, project)
}

func (store *testPolicyStore) UpdateProjectRetention(_ context.Context, _ string, days *int32, _ auth.Actor) error {
	if store.updateError != nil {
		return store.updateError
	}
	store.days = days
	return nil
}

func TestPolicyUpdateInvalidatesOnlyOnSuccess(t *testing.T) {
	ctx := context.Background()
	const day = 24 * time.Hour
	serviceTime := testNow
	wantError := errors.New("update failed")
	store := &testPolicyStore{updateError: wantError}
	service := NewPolicyService(store, 7*day, time.Minute, 16, func() time.Time { return serviceTime })
	checkCutoff := func(age time.Duration, wantReads int) {
		t.Helper()
		cutoff, err := service.Cutoff(ctx, "project")
		if want := serviceTime.Add(-age); err != nil || !cutoff.Equal(want) || store.calls != wantReads {
			t.Fatalf("cutoff=%v reads=%d err=%v, want %v with %d reads", cutoff, store.calls, err, want, wantReads)
		}
	}
	checkCutoff(7*day, 1)
	serviceTime = serviceTime.Add(time.Second)
	one := int32(1)
	if err := service.UpdateProjectRetention(ctx, "project", &one, auth.Actor{}); !errors.Is(err, wantError) {
		t.Fatalf("update error = %v, want %v", err, wantError)
	}
	checkCutoff(7*day, 1)
	store.updateError = nil
	if err := service.UpdateProjectRetention(ctx, "project", &one, auth.Actor{}); err != nil {
		t.Fatal(err)
	}
	checkCutoff(day, 2)
}
