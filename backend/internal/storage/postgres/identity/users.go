package identity

import (
	"context"
	"errors"
	"fmt"
	"github.com/vanquan3008/tracepilot/internal/xslices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func insertUser(ctx context.Context, queries *sqlcstore.Queries, params auth.CreateUserParams) (auth.User, error) {
	hash, hashError := auth.HashPassword(params.Password)
	if hashError != nil {
		return auth.User{}, hashError
	}
	row, insertError := queries.InsertUser(ctx, sqlcstore.InsertUserParams{
		ID: newID(), Email: params.Email, DisplayName: params.DisplayName, PasswordHash: hash,
		PlatformAdmin: params.PlatformAdmin, MustChangePassword: params.MustChangePassword,
	})
	if pgvalue.IsUniqueViolationOf(insertError, singlePlatformAdminIndex) {
		return auth.User{}, auth.ErrPlatformAdminExists
	}
	if pgvalue.IsUniqueViolation(insertError) {
		return auth.User{}, auth.ErrEmailExists
	}
	if insertError != nil {
		return auth.User{}, fmt.Errorf("insert user: %w", insertError)
	}
	return userFrom(row), nil
}

func (store *IdentityStore) CreateUser(ctx context.Context, params auth.CreateUserParams, actor auth.Actor) (auth.User, error) {
	params, validationError := auth.NormalizeCreateUserParams(params)
	if validationError != nil {
		return auth.User{}, validationError
	}
	var user auth.User
	createError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		if params.PlatformAdmin {
			if lockError := queries.AcquireTransactionLock(ctx, adminLockKey); lockError != nil {
				return lockError
			}
			exists, existsError := queries.PlatformAdminExists(ctx)
			if existsError != nil {
				return existsError
			}
			if exists {
				return auth.ErrPlatformAdminExists
			}
		}
		created, insertError := insertUser(ctx, queries, params)
		if insertError != nil {
			return insertError
		}
		user = created
		return audit(ctx, queries, "", actor, "user.created", "user", created.ID, map[string]any{"email": created.Email, "platform_admin": created.PlatformAdmin})
	})
	if createError != nil {
		return auth.User{}, fmt.Errorf("create user: %w", createError)
	}
	return user, nil
}

func (store *IdentityStore) GetUser(ctx context.Context, userID string) (auth.User, error) {
	row, queryError := store.queries.GetUser(ctx, userID)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrUserNotFound
	}
	if queryError != nil {
		return auth.User{}, fmt.Errorf("get user: %w", queryError)
	}
	return userFrom(row), nil
}

func (store *IdentityStore) GetUserByEmail(ctx context.Context, email string) (auth.User, error) {
	credentials, lookupError := store.GetCredentials(ctx, email)
	return credentials.User, lookupError
}

// GetCredentials loads an account with its password hash for verification.
func (store *IdentityStore) GetCredentials(ctx context.Context, email string) (auth.Credentials, error) {
	email, emailError := auth.NormalizeEmail(email)
	if emailError != nil {
		return auth.Credentials{}, auth.ErrUserNotFound
	}
	row, queryError := store.queries.GetUserByEmail(ctx, email)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.Credentials{}, auth.ErrUserNotFound
	}
	if queryError != nil {
		return auth.Credentials{}, fmt.Errorf("get credentials: %w", queryError)
	}
	return auth.Credentials{User: userFrom(row), PasswordHash: row.PasswordHash}, nil
}

func (store *IdentityStore) GetCredentialsByID(ctx context.Context, userID string) (auth.Credentials, error) {
	row, queryError := store.queries.GetUser(ctx, userID)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.Credentials{}, auth.ErrUserNotFound
	}
	if queryError != nil {
		return auth.Credentials{}, fmt.Errorf("get credentials: %w", queryError)
	}
	return auth.Credentials{User: userFrom(row), PasswordHash: row.PasswordHash}, nil
}

func (store *IdentityStore) RecordLogin(ctx context.Context, userID string) error {
	if updateError := store.queries.RecordUserLogin(ctx, userID); updateError != nil {
		return fmt.Errorf("record login: %w", updateError)
	}
	return nil
}

func (store *IdentityStore) ListUsers(ctx context.Context, filter auth.UserFilter) ([]auth.User, error) {
	rows, queryError := store.queries.ListUsers(ctx, sqlcstore.ListUsersParams{
		Before: pgvalue.TimeOrNil(filter.Before), CursorID: pgvalue.OrNil(filter.CursorID), RowLimit: int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list users: %w", queryError)
	}
	return xslices.Map(rows, userFrom), nil
}

// UpdateUser applies the set fields. The super administrator cannot be disabled, and disabling
// an account ends its sessions.
func (store *IdentityStore) UpdateUser(ctx context.Context, userID string, update auth.UpdateUserParams, actor auth.Actor) (auth.User, error) {
	if update.DisplayName != nil {
		name, nameError := auth.NormalizeDisplayName(*update.DisplayName)
		if nameError != nil {
			return auth.User{}, nameError
		}
		update.DisplayName = &name
	}
	var user auth.User
	updateError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		current, loadError := queries.LockUser(ctx, userID)
		if errors.Is(loadError, pgx.ErrNoRows) {
			return auth.ErrUserNotFound
		}
		if loadError != nil {
			return loadError
		}
		if current.PlatformAdmin && update.Disabled != nil && *update.Disabled {
			return auth.ErrLastPlatformAdmin
		}
		updated, saveError := queries.UpdateUser(ctx, sqlcstore.UpdateUserParams{
			ID: userID, DisplayName: update.DisplayName, Disabled: update.Disabled, ValidAfter: time.Now().UTC(),
		})
		if saveError != nil {
			return saveError
		}
		if updated.DisabledAt != nil {
			if revokeError := queries.DeleteUserSessions(ctx, userID); revokeError != nil {
				return revokeError
			}
		}
		user = userFrom(updated)
		return audit(ctx, queries, "", actor, "user.updated", "user", userID, map[string]any{
			"platform_admin": updated.PlatformAdmin, "disabled": updated.DisabledAt != nil,
		})
	})
	if updateError != nil {
		return auth.User{}, fmt.Errorf("update user: %w", updateError)
	}
	return user, nil
}

// SetPassword replaces an account's password and ends its other sessions. keepToken, when set,
// names the session the change came from so the person stays signed in.
func (store *IdentityStore) SetPassword(ctx context.Context, userID, newPassword string, mustChange bool, keepToken string, actor auth.Actor) error {
	if validationError := auth.ValidatePassword(newPassword); validationError != nil {
		return fmt.Errorf("%w: %v", auth.ErrInvalidIdentity, validationError)
	}
	hash, hashError := auth.HashPassword(newPassword)
	if hashError != nil {
		return hashError
	}
	setError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		changed, updateError := queries.SetUserPassword(ctx, sqlcstore.SetUserPasswordParams{ID: userID, PasswordHash: hash, MustChangePassword: mustChange, ValidAfter: time.Now().UTC()})
		if updateError != nil {
			return updateError
		}
		if changed == 0 {
			return auth.ErrUserNotFound
		}
		var revokeError error
		if keepToken != "" {
			revokeError = queries.DeleteUserSessionsExcept(ctx, sqlcstore.DeleteUserSessionsExceptParams{UserID: userID, KeepTokenHash: hashToken(keepToken)})
		} else {
			revokeError = queries.DeleteUserSessions(ctx, userID)
		}
		if revokeError != nil {
			return revokeError
		}
		return audit(ctx, queries, "", actor, "user.password_set", "user", userID, map[string]any{"must_change": mustChange})
	})
	if setError != nil {
		return fmt.Errorf("set password: %w", setError)
	}
	return nil
}
