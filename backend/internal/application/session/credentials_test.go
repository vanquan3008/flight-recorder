package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type credentialStore struct {
	credentials auth.Credentials
	loadError   error
	writeError  error
	email       string
	userID      string
	writes      []ChangePasswordCommand
}

func (store *credentialStore) GetCredentials(_ context.Context, email string) (auth.Credentials, error) {
	store.email = email
	return store.credentials, store.loadError
}

func (store *credentialStore) GetCredentialsByID(_ context.Context, userID string) (auth.Credentials, error) {
	store.userID = userID
	return store.credentials, store.loadError
}

func (store *credentialStore) SetPassword(_ context.Context, userID, password string, mustChange bool, keep string, actor auth.Actor) error {
	if mustChange {
		return errors.New("a self-service password change must end the temporary-password restriction")
	}
	store.writes = append(store.writes, ChangePasswordCommand{UserID: userID, NewPassword: password, RefreshToken: keep, Actor: actor})
	return store.writeError
}

func passwordHash(t *testing.T) string {
	t.Helper()
	hash, err := auth.HashPassword("current-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestAuthenticate(t *testing.T) {
	hash := passwordHash(t)
	now := time.Now()
	unavailable := errors.New("database unavailable")
	for _, test := range []struct {
		name      string
		password  string
		loadError error
		disabled  *time.Time
		wantError error
	}{
		{name: "normalizes email", password: "current-passphrase"},
		{name: "unknown email", password: "current-passphrase", loadError: auth.ErrUserNotFound, wantError: auth.ErrInvalidCredentials},
		{name: "wrong password", password: "wrong-passphrase", wantError: auth.ErrInvalidCredentials},
		{name: "disabled account", password: "current-passphrase", disabled: &now, wantError: auth.ErrInvalidCredentials},
		{name: "oversized password", password: strings.Repeat("x", auth.MaxPasswordLength*4+1), wantError: auth.ErrInvalidCredentials},
		{name: "storage failure", password: "current-passphrase", loadError: unavailable, wantError: unavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &credentialStore{
				credentials: auth.Credentials{User: auth.User{ID: "user", DisabledAt: test.disabled}, PasswordHash: hash},
				loadError:   test.loadError,
			}
			user, err := Authenticate(context.Background(), store, "  ALICE@EXAMPLE.COM ", test.password)
			if !errors.Is(err, test.wantError) || store.email != "alice@example.com" {
				t.Fatalf("email = %q, error = %v, want %v", store.email, err, test.wantError)
			}
			if err == nil && user.ID != "user" || err != nil && user.ID != "" {
				t.Fatalf("authentication returned an unexpected user: %+v", user)
			}
		})
	}
}

func TestChangePasswordOnlyWritesAfterVerification(t *testing.T) {
	hash := passwordHash(t)
	unavailable := errors.New("database unavailable")
	for _, test := range []struct {
		name       string
		current    string
		next       string
		loadError  error
		writeError error
		wantError  error
		wantWrites int
	}{
		{name: "success", current: "current-passphrase", next: "replacement-passphrase", wantWrites: 1},
		{name: "wrong password", current: "incorrect", next: "replacement-passphrase", wantError: auth.ErrInvalidCredentials},
		{name: "oversized password", current: strings.Repeat("x", auth.MaxPasswordLength*4+1), wantError: auth.ErrInvalidCredentials},
		{name: "same password", current: "current-passphrase", next: "current-passphrase", wantError: auth.ErrInvalidIdentity},
		{name: "lookup failure", loadError: unavailable, wantError: unavailable},
		{name: "write failure", current: "current-passphrase", next: "replacement-passphrase", writeError: unavailable, wantError: unavailable, wantWrites: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &credentialStore{credentials: auth.Credentials{PasswordHash: hash}, loadError: test.loadError, writeError: test.writeError}
			command := ChangePasswordCommand{
				UserID: "user", CurrentPassword: test.current, NewPassword: test.next,
				RefreshToken: "current-browser", Actor: auth.Actor{UserID: "user"},
			}
			err := ChangePassword(context.Background(), store, command)
			if !errors.Is(err, test.wantError) || len(store.writes) != test.wantWrites || store.userID != "user" {
				t.Fatalf("error = %v, writes = %d, user = %q", err, len(store.writes), store.userID)
			}
			if len(store.writes) > 0 {
				written := store.writes[0]
				if written.UserID != command.UserID || written.NewPassword != command.NewPassword || written.RefreshToken != command.RefreshToken || written.Actor != command.Actor {
					t.Fatalf("lost password, session or audit context: %+v", written)
				}
			}
		})
	}
}
