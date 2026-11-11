package main

import (
	"regexp"
	"strings"
	"testing"
)

var seedUUIDPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)

func TestRenderSeedSQLCreatesOnlySuperAdmin(t *testing.T) {
	t.Parallel()

	sql, err := renderSeedSQL()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"INSERT INTO users (id, email, display_name, password_hash, platform_admin)",
		adminEmail,
		"'Admin'",
		"true",
		"ON CONFLICT (email) DO NOTHING",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("rendered seed SQL does not contain %q", want)
		}
	}
	for _, forbidden := range []string{
		"tracepilot-sample",
		"INSERT INTO projects",
		"INSERT INTO environments",
		"INSERT INTO project_members",
		"INSERT INTO services",
		"INSERT INTO events",
		"INSERT INTO spans",
		"INSERT INTO logs",
		"INSERT INTO incidents",
	} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("development account seed unexpectedly contains %q", forbidden)
		}
	}
	if strings.Contains(sql, "{{") || strings.Contains(sql, "}}") {
		t.Fatal("rendered seed SQL contains an unresolved placeholder")
	}

	ids := seedUUIDs(sql)
	if len(ids) != 1 {
		t.Fatalf("rendered admin seed has %d unique UUIDs, want only the admin", len(ids))
	}
}

func TestRenderSeedSQLGeneratesDifferentAdminIDs(t *testing.T) {
	t.Parallel()

	first, err := renderSeedSQL()
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderSeedSQL()
	if err != nil {
		t.Fatal(err)
	}

	firstIDs := seedUUIDs(first)
	secondIDs := seedUUIDs(second)
	if len(firstIDs) != 1 || len(secondIDs) != 1 {
		t.Fatalf("generated admin ID counts = %d and %d, want one per seed", len(firstIDs), len(secondIDs))
	}
	for id := range firstIDs {
		if _, reused := secondIDs[id]; reused {
			t.Fatalf("seed runs reused admin UUID %q", id)
		}
	}
}

func TestNewSeedUUIDsAreV4AndUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 100)
	for range 100 {
		id, err := newSeedUUID()
		if err != nil {
			t.Fatal(err)
		}
		if !seedUUIDPattern.MatchString(id) {
			t.Fatalf("generated UUID is not a canonical UUID v4: %q", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate UUID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func seedUUIDs(sql string) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, id := range seedUUIDPattern.FindAllString(sql, -1) {
		ids[id] = struct{}{}
	}
	return ids
}
