package identity

import (
	"context"
	"errors"
	"fmt"
	"github.com/vanquan3008/tracepilot/internal/xslices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// CreateProject adds a project with its first environment. creatorUserID, when set, becomes its owner;
// ownedLimit, when positive, caps how many projects that person may own.
func (store *IdentityStore) CreateProject(ctx context.Context, params auth.CreateProjectParams, creatorUserID string, ownedLimit int, actor auth.Actor) (auth.Project, auth.Environment, error) {
	params, validationError := auth.NormalizeCreateProjectParams(params)
	if validationError != nil {
		return auth.Project{}, auth.Environment{}, validationError
	}
	var project auth.Project
	var environment auth.Environment
	createError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		if creatorUserID != "" && ownedLimit > 0 {
			if lockError := queries.AcquireTransactionLock(ctx, membershipLockKey); lockError != nil {
				return lockError
			}
			owned, countError := queries.CountOwnedProjects(ctx, creatorUserID)
			if countError != nil {
				return countError
			}
			if owned >= int64(ownedLimit) {
				return auth.ErrTooManyProjects
			}
		}
		created, insertError := queries.InsertProject(ctx, sqlcstore.InsertProjectParams{ID: newID(), Slug: params.Slug, Name: params.Name})
		if pgvalue.IsUniqueViolation(insertError) {
			return auth.ErrProjectSlugExists
		}
		if insertError != nil {
			return insertError
		}
		project = projectFrom(created)
		first, environmentError := queries.InsertEnvironment(ctx, sqlcstore.InsertEnvironmentParams{ID: newID(), ProjectID: project.ID, Name: params.EnvironmentName})
		if environmentError != nil {
			return environmentError
		}
		environment = environmentFrom(first)
		if creatorUserID != "" {
			if memberError := queries.InsertProjectMember(ctx, sqlcstore.InsertProjectMemberParams{ProjectID: project.ID, UserID: creatorUserID, Role: auth.RoleOwner}); memberError != nil {
				return memberError
			}
		}
		return audit(ctx, queries, project.ID, actor, "project.created", "project", project.ID, map[string]any{"slug": project.Slug})
	})
	if createError != nil {
		return auth.Project{}, auth.Environment{}, fmt.Errorf("create project: %w", createError)
	}
	return project, environment, nil
}

// GetProject finds a project by ID or slug.
func (store *IdentityStore) GetProject(ctx context.Context, reference string) (auth.Project, error) {
	row, queryError := store.queries.GetProjectByReference(ctx, reference)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return auth.Project{}, auth.ErrProjectNotFound
	}
	if queryError != nil {
		return auth.Project{}, fmt.Errorf("get project: %w", queryError)
	}
	return projectFrom(row), nil
}

func (store *IdentityStore) RenameProject(ctx context.Context, projectID, name string, actor auth.Actor) (auth.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return auth.Project{}, fmt.Errorf("%w: project name must be 1-100 characters", auth.ErrInvalidIdentity)
	}
	var project auth.Project
	renameError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		renamed, updateError := queries.RenameProject(ctx, sqlcstore.RenameProjectParams{ID: projectID, Name: name})
		if errors.Is(updateError, pgx.ErrNoRows) {
			return auth.ErrProjectNotFound
		}
		if updateError != nil {
			return updateError
		}
		project = projectFrom(renamed)
		return audit(ctx, queries, projectID, actor, "project.renamed", "project", projectID, map[string]any{"name": name})
	})
	if renameError != nil {
		return auth.Project{}, fmt.Errorf("rename project: %w", renameError)
	}
	return project, nil
}

// ListMemberships returns the projects a user belongs to with their role in each.
func (store *IdentityStore) ListMemberships(ctx context.Context, userID string) ([]auth.Membership, error) {
	rows, queryError := store.queries.ListUserProjects(ctx, userID)
	if queryError != nil {
		return nil, fmt.Errorf("list projects: %w", queryError)
	}
	memberships := make([]auth.Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, auth.Membership{Project: projectFrom(row.Project), Role: row.Role})
	}
	return memberships, nil
}

// ListAllProjects returns every project as an owner membership, for platform administrators.
func (store *IdentityStore) ListAllProjects(ctx context.Context) ([]auth.Membership, error) {
	rows, queryError := store.queries.ListAllProjects(ctx)
	if queryError != nil {
		return nil, fmt.Errorf("list projects: %w", queryError)
	}
	memberships := make([]auth.Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, auth.Membership{Project: projectFrom(row), Role: auth.RoleOwner})
	}
	return memberships, nil
}

// MemberRole returns a user's role in a project, or "" when they are not a member.
func (store *IdentityStore) MemberRole(ctx context.Context, projectID, userID string) (string, error) {
	role, queryError := store.queries.GetMemberRole(ctx, sqlcstore.GetMemberRoleParams{ProjectID: projectID, UserID: userID})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return "", nil
	}
	if queryError != nil {
		return "", fmt.Errorf("get member role: %w", queryError)
	}
	return role, nil
}

func (store *IdentityStore) ListMembers(ctx context.Context, projectID string) ([]auth.Member, error) {
	rows, queryError := store.queries.ListProjectMembers(ctx, projectID)
	if queryError != nil {
		return nil, fmt.Errorf("list members: %w", queryError)
	}
	members := make([]auth.Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, auth.Member{User: userFrom(row.User), Role: row.Role, CreatedAt: row.MemberSince})
	}
	return members, nil
}

// AddMember grants an existing account a role in a project.
func (store *IdentityStore) AddMember(ctx context.Context, projectID, userID, role string, actor auth.Actor) error {
	if !auth.ValidRole(role) {
		return fmt.Errorf("%w: role must be one of owner, user", auth.ErrInvalidIdentity)
	}
	addError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		if insertError := queries.InsertProjectMember(ctx, sqlcstore.InsertProjectMemberParams{ProjectID: projectID, UserID: userID, Role: role}); insertError != nil {
			if pgvalue.IsUniqueViolation(insertError) {
				return auth.ErrMemberExists
			}
			if pgvalue.IsForeignKeyViolation(insertError) {
				return auth.ErrUserNotFound
			}
			return insertError
		}
		return audit(ctx, queries, projectID, actor, "member.added", "user", userID, map[string]any{"role": role})
	})
	if addError != nil {
		return fmt.Errorf("add member: %w", addError)
	}
	return nil
}

// ChangeMemberRole sets a member's role. allowed receives the current role and may refuse the
// change; it runs under the membership lock so the decision and the write cannot interleave.
func (store *IdentityStore) ChangeMemberRole(ctx context.Context, projectID, userID, role string, actor auth.Actor, allowed func(current string) bool) error {
	if !auth.ValidRole(role) {
		return fmt.Errorf("%w: role must be one of owner, user", auth.ErrInvalidIdentity)
	}
	changeError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		current, guardError := guardMembership(ctx, queries, projectID, userID, allowed, role != auth.RoleOwner)
		if guardError != nil {
			return guardError
		}
		if updateError := queries.UpdateProjectMemberRole(ctx, sqlcstore.UpdateProjectMemberRoleParams{ProjectID: projectID, UserID: userID, Role: role}); updateError != nil {
			return updateError
		}
		return audit(ctx, queries, projectID, actor, "member.role_changed", "user", userID, map[string]any{"from": current, "to": role})
	})
	if changeError != nil {
		return fmt.Errorf("change member role: %w", changeError)
	}
	return nil
}

// RemoveMember ends a user's access to a project. allowed behaves as in ChangeMemberRole.
func (store *IdentityStore) RemoveMember(ctx context.Context, projectID, userID string, actor auth.Actor, allowed func(current string) bool) error {
	removeError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		current, guardError := guardMembership(ctx, queries, projectID, userID, allowed, true)
		if guardError != nil {
			return guardError
		}
		if deleteError := queries.DeleteProjectMember(ctx, sqlcstore.DeleteProjectMemberParams{ProjectID: projectID, UserID: userID}); deleteError != nil {
			return deleteError
		}
		return audit(ctx, queries, projectID, actor, "member.removed", "user", userID, map[string]any{"role": current})
	})
	if removeError != nil {
		return fmt.Errorf("remove member: %w", removeError)
	}
	return nil
}

// guardMembership locks a membership and returns its current role. It refuses a change allowed
// vetoes, and, when the member would stop being an owner, one that leaves the project ownerless.
func guardMembership(ctx context.Context, queries *sqlcstore.Queries, projectID, userID string, allowed func(current string) bool, losesOwnership bool) (string, error) {
	if lockError := queries.AcquireTransactionLock(ctx, membershipLockKey); lockError != nil {
		return "", lockError
	}
	current, loadError := queries.LockMemberRole(ctx, sqlcstore.LockMemberRoleParams{ProjectID: projectID, UserID: userID})
	if errors.Is(loadError, pgx.ErrNoRows) {
		return "", auth.ErrMemberNotFound
	}
	if loadError != nil {
		return "", loadError
	}
	if allowed != nil && !allowed(current) {
		return "", auth.ErrForbiddenRoleChange
	}
	if current == auth.RoleOwner && losesOwnership {
		others, countError := queries.CountOtherActiveOwners(ctx, sqlcstore.CountOtherActiveOwnersParams{ProjectID: projectID, UserID: userID})
		if countError != nil {
			return "", countError
		}
		if others == 0 {
			return "", auth.ErrLastOwner
		}
	}
	return current, nil
}

func (store *IdentityStore) ListEnvironments(ctx context.Context, projectID string) ([]auth.Environment, error) {
	rows, queryError := store.queries.ListEnvironments(ctx, projectID)
	if queryError != nil {
		return nil, fmt.Errorf("list environments: %w", queryError)
	}
	return xslices.Map(rows, environmentFrom), nil
}

// ListScopes returns every environment of every project.
func (store *IdentityStore) ListScopes(ctx context.Context) ([]auth.Scope, error) {
	rows, queryError := store.queries.ListScopes(ctx)
	if queryError != nil {
		return nil, fmt.Errorf("list scopes: %w", queryError)
	}
	scopes := make([]auth.Scope, 0, len(rows))
	for _, row := range rows {
		scopes = append(scopes, auth.Scope{Project: projectFrom(row.Project), Environment: environmentFrom(row.Environment)})
	}
	return scopes, nil
}

// CreateEnvironment adds an environment to a project, such as staging next to production.
func (store *IdentityStore) CreateEnvironment(ctx context.Context, projectID, name string, actor auth.Actor) (auth.Environment, error) {
	name, nameError := auth.NormalizeEnvironmentName(name)
	if nameError != nil {
		return auth.Environment{}, nameError
	}
	var environment auth.Environment
	createError := store.inTransaction(ctx, func(queries *sqlcstore.Queries) error {
		created, insertError := queries.InsertEnvironment(ctx, sqlcstore.InsertEnvironmentParams{ID: newID(), ProjectID: projectID, Name: name})
		if pgvalue.IsUniqueViolation(insertError) {
			return auth.ErrEnvironmentExists
		}
		if pgvalue.IsForeignKeyViolation(insertError) {
			return auth.ErrProjectNotFound
		}
		if insertError != nil {
			return insertError
		}
		environment = environmentFrom(created)
		return audit(ctx, queries, projectID, actor, "environment.created", "environment", environment.ID, map[string]any{"name": name})
	})
	if createError != nil {
		return auth.Environment{}, fmt.Errorf("create environment: %w", createError)
	}
	return environment, nil
}
