package auth

// Scopes that never appear on telemetry senders: they describe a signed-in person.
const (
	// ScopeSession is any signed-in user, with or without a project.
	ScopeSession = "session"
	// ScopePlatform is a super administrator: manages accounts and sees every project.
	ScopePlatform = "platform"
)

// Three kinds of people use TracePilot.
//
//	super administrator  a platform flag on the account: manages accounts, opens every project as owner
//	owner                made a project to receive its telemetry; runs it and decides who may see it
//	user                 was allowed into a project by its owner; reads telemetry and works incidents
//
// Owner and user are the two roles a project member holds.
const (
	RoleOwner = "owner"
	RoleUser  = "user"
)

// Roles lists project roles from most to least privileged.
var Roles = []string{RoleOwner, RoleUser}

// ValidRole reports whether role is one a member can hold.
func ValidRole(role string) bool {
	for _, known := range Roles {
		if role == known {
			return true
		}
	}
	return false
}

// ScopesForRole maps a project role to the scopes its routes require.
//
//	user   read, write      see telemetry, change incident status
//	owner  everything       also replay, ingest tokens, webhooks, members and project settings
//
// Replay sends a request from the server to a configured target, so it stays with owners even
// though it only reads incident data.
func ScopesForRole(role string) []string {
	switch role {
	case RoleOwner:
		return []string{ScopeAdmin, ScopeRead, ScopeReplay, ScopeWrite}
	case RoleUser:
		return []string{ScopeRead, ScopeWrite}
	default:
		return nil
	}
}

// CanManageRole reports whether an actor holding actorRole may grant, change or remove a
// membership held at targetRole. Only owners manage membership.
func CanManageRole(actorRole, targetRole string) bool {
	return actorRole == RoleOwner && ValidRole(targetRole)
}
