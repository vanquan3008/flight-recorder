package httpapi

import (
	"github.com/vanquan3008/tracepilot/internal/auth"
	"sync"
	"time"
)

const (
	defaultIdentityCacheTTL   = 30 * time.Second
	defaultIdentityCacheLimit = 1024
)

type cachedUserEntry struct {
	user     auth.User
	loadedAt time.Time
}

type cachedProjectEntry struct {
	project  auth.Project
	loadedAt time.Time
}

type cachedRoleEntry struct {
	role     string
	loadedAt time.Time
}

type identityCache struct {
	mu       sync.Mutex
	users    map[string]cachedUserEntry
	projects map[string]cachedProjectEntry
	roles    map[string]cachedRoleEntry
	ttl      time.Duration
	limit    int
}

func newIdentityCache(ttl time.Duration, limit int) *identityCache {
	if ttl <= 0 {
		ttl = defaultIdentityCacheTTL
	}
	if limit <= 0 {
		limit = defaultIdentityCacheLimit
	}
	return &identityCache{
		users:    make(map[string]cachedUserEntry),
		projects: make(map[string]cachedProjectEntry),
		roles:    make(map[string]cachedRoleEntry),
		ttl:      ttl,
		limit:    limit,
	}
}

func (cache *identityCache) getUser(id string, now time.Time) (auth.User, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.users[id]
	return entry.user, ok && now.Sub(entry.loadedAt) < cache.ttl
}

func (cache *identityCache) putUser(id string, user auth.User, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.users) >= cache.limit {
		cache.users = make(map[string]cachedUserEntry)
	}
	cache.users[id] = cachedUserEntry{user: user, loadedAt: now}
}

func (cache *identityCache) forgetUser(id string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.users, id)
}

func (cache *identityCache) getProject(ref string, now time.Time) (auth.Project, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.projects[ref]
	return entry.project, ok && now.Sub(entry.loadedAt) < cache.ttl
}

func (cache *identityCache) putProject(ref string, project auth.Project, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.projects) >= cache.limit {
		cache.projects = make(map[string]cachedProjectEntry)
	}
	entry := cachedProjectEntry{project: project, loadedAt: now}
	cache.projects[ref] = entry
	cache.projects[project.ID] = entry
	if project.Slug != "" {
		cache.projects[project.Slug] = entry
	}
}

func (cache *identityCache) forgetProject(id, slug string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.projects, id)
	if slug != "" {
		delete(cache.projects, slug)
	}
}

func (cache *identityCache) getRole(projectID, userID string, now time.Time) (string, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.roles[projectID+":"+userID]
	return entry.role, ok && now.Sub(entry.loadedAt) < cache.ttl
}

func (cache *identityCache) putRole(projectID, userID, role string, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.roles) >= cache.limit {
		cache.roles = make(map[string]cachedRoleEntry)
	}
	cache.roles[projectID+":"+userID] = cachedRoleEntry{role: role, loadedAt: now}
}

func (cache *identityCache) forgetRole(projectID, userID string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.roles, projectID+":"+userID)
}
