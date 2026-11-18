package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

func TestSeedAdminParamsDescribeOnlyTheDocumentedSuperAdmin(t *testing.T) {
	params, err := auth.NormalizeCreateUserParams(seedAdminParams())
	if err != nil {
		t.Fatalf("seed account fails account validation: %v", err)
	}
	if params.Email != adminEmail || params.Password != "admin-password-123" || !params.PlatformAdmin || params.MustChangePassword {
		t.Fatalf("seed account = %+v, want the documented super admin", params)
	}
}

func TestSeedAdminKeepsAnExistingAccount(t *testing.T) {
	unavailable := errors.New("database unavailable")
	for name, test := range map[string]struct {
		result      error
		wantCreated bool
		wantErr     error
	}{
		"new account":       {nil, true, nil},
		"existing account":  {fmt.Errorf("create user: %w", auth.ErrEmailExists), false, nil},
		"database failure":  {unavailable, false, unavailable},
		"invalid parameter": {auth.ErrInvalidIdentity, false, auth.ErrInvalidIdentity},
	} {
		t.Run(name, func(t *testing.T) {
			var calls int
			created, err := seedAdmin(context.Background(), func(_ context.Context, params auth.CreateUserParams, actor auth.Actor) (auth.User, error) {
				calls++
				if params != seedAdminParams() || actor != (auth.Actor{}) {
					t.Fatalf("seed created %+v as %+v", params, actor)
				}
				return auth.User{}, test.result
			})
			if calls != 1 || created != test.wantCreated || !errors.Is(err, test.wantErr) || (test.wantErr == nil) != (err == nil) {
				t.Fatalf("seedAdmin = %t, %v after %d calls; want %t, %v", created, err, calls, test.wantCreated, test.wantErr)
			}
		})
	}
}
