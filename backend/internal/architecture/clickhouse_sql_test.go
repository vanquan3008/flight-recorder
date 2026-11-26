package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Operational schema/retention SQL and the HTTP client have different concerns
// from application queries. Short predicates and column lists can stay in Go.
func TestClickHouseMultilineSQLLivesInEmbeddedFiles(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "storage", "clickhouse")
	statement := regexp.MustCompile(`(?i)^\s*(SELECT|WITH|INSERT|UPDATE|DELETE|FROM|JOIN|CREATE|ALTER)\b`)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "client", "clienttest", "integration", "schema", "retention":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.TrimSpace(value), "\n") && statement.MatchString(value) {
				t.Errorf("%s: move multiline ClickHouse SQL to sql/*.sql and embed it in queries.go", positions.Position(literal.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
