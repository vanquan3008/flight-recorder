// Package session owns credential verification and password changes. HTTP
// adapters retain cookies, token issuance, rate limits and response contracts.
package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/auth"
)

type CredentialReader interface {
	GetCredentials(context.Context, string) (auth.Credentials, error)
}

type PasswordStore interface {
	GetCredentialsByID(context.Context, string) (auth.Credentials, error)
	SetPassword(context.Context, string, string, bool, string, auth.Actor) error
}

func Authenticate(ctx context.Context, store CredentialReader, email, password string) (auth.User, error) {
	credentials, err := store.GetCredentials(ctx, strings.ToLower(strings.TrimSpace(email)))
	hash := auth.DummyPasswordHash()
	if err == nil {
		hash = credentials.PasswordHash
	} else if !errors.Is(err, auth.ErrUserNotFound) {
		return auth.User{}, fmt.Errorf("load credentials: %w", err)
	}
	// Unknown accounts still verify a hash so timing does not reveal registered emails.
	valid := len(password) <= auth.MaxPasswordLength*4 && auth.VerifyPassword(password, hash)
	if err != nil || !valid || credentials.User.DisabledAt != nil {
		return auth.User{}, auth.ErrInvalidCredentials
	}
	return credentials.User, nil
}

type ChangePasswordCommand struct {
	UserID          string
	CurrentPassword string
	NewPassword     string
	RefreshToken    string
	Actor           auth.Actor
}

func ChangePassword(ctx context.Context, store PasswordStore, command ChangePasswordCommand) error {
	credentials, err := store.GetCredentialsByID(ctx, command.UserID)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if len(command.CurrentPassword) > auth.MaxPasswordLength*4 || !auth.VerifyPassword(command.CurrentPassword, credentials.PasswordHash) {
		return auth.ErrInvalidCredentials
	}
	if command.NewPassword == command.CurrentPassword {
		return fmt.Errorf("%w: choose a password different from the current one", auth.ErrInvalidIdentity)
	}
	// The store atomically changes the password and revokes all other sessions/tokens.
	if err := store.SetPassword(ctx, command.UserID, command.NewPassword, false, command.RefreshToken, command.Actor); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}
