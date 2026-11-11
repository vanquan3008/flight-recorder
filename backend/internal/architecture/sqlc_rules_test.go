package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func TestSQLCMutationRuleAllowsUpserts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	configuration, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "sqlc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	patterns := regexp.MustCompile(`query\.sql\.matches\(("(?:[^"\\]|\\.)*")\)`).FindAllSubmatch(configuration, -1)
	if len(patterns) != 2 {
		t.Fatal("expected mutation and WHERE patterns in the sqlc rule")
	}
	var compiled []*regexp.Regexp
	for _, pattern := range patterns {
		source, err := strconv.Unquote(string(pattern[1]))
		if err != nil {
			t.Fatal(err)
		}
		compiled = append(compiled, regexp.MustCompile(source))
	}
	for _, test := range []struct {
		name, sql string
		unsafe    bool
	}{
		{"update all", "UPDATE projects SET name = $1", true},
		{"update alias", "UPDATE projects AS p SET name = $1", true},
		{"scoped update", "UPDATE projects SET name = $1 WHERE id = $2", false},
		{"delete all", "DELETE FROM projects", true},
		{"scoped delete", "DELETE FROM projects where id = $1", false},
		{"cte delete", "WITH removed AS (DELETE FROM projects RETURNING id) SELECT * FROM removed", true},
		{"upsert", "INSERT INTO projects(id) VALUES ($1) ON CONFLICT(id) DO UPDATE SET name = EXCLUDED.name", false},
		{"cte upsert", "WITH project AS (INSERT INTO projects(id) VALUES ($1) ON CONFLICT(id) DO UPDATE SET name = EXCLUDED.name RETURNING id) INSERT INTO environments SELECT id FROM project", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			unsafe := compiled[0].MatchString(test.sql) && !compiled[1].MatchString(test.sql)
			if unsafe != test.unsafe {
				t.Fatalf("unsafe = %t, want %t", unsafe, test.unsafe)
			}
		})
	}
}
