package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
)

func TestSeedAdminParamsUsesConfiguredSeedData(t *testing.T) {
	cfg := config.SeedConfig{
		AdminEmail:    "admin@tracepilot.test",
		AdminPassword: "admin-password-test-123",
		AdminName:     "Test Admin",
	}
	raw, err := seedAdminParams(cfg)
	if err != nil {
		t.Fatalf("seedAdminParams failed: %v", err)
	}
	params, err := auth.NormalizeCreateUserParams(raw)
	if err != nil {
		t.Fatalf("seed account fails account validation: %v", err)
	}
	if params.Email != cfg.AdminEmail || params.Password != cfg.AdminPassword || params.DisplayName != cfg.AdminName || !params.PlatformAdmin || params.MustChangePassword {
		t.Fatalf("seed account = %+v, want configured super admin", params)
	}
}

func TestSeedAdminParamsRequiresEmailAndPassword(t *testing.T) {
	if _, err := seedAdminParams(config.SeedConfig{AdminPassword: "password-123"}); err == nil {
		t.Fatal("expected error when email is missing")
	}
	if _, err := seedAdminParams(config.SeedConfig{AdminEmail: "admin@test.local"}); err == nil {
		t.Fatal("expected error when password is missing")
	}
}

func TestSeedAdminKeepsAnExistingAccount(t *testing.T) {
	cfg := config.SeedConfig{
		AdminEmail:    "admin@tracepilot.test",
		AdminPassword: "admin-password-test-123",
		AdminName:     "Admin",
	}
	expectedParams, err := seedAdminParams(cfg)
	if err != nil {
		t.Fatalf("seedAdminParams failed: %v", err)
	}

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
			created, err := seedAdmin(context.Background(), cfg, func(_ context.Context, params auth.CreateUserParams, actor auth.Actor) (auth.User, error) {
				calls++
				if params != expectedParams || actor != (auth.Actor{}) {
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
