package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// handwrittenSQL lists the only storage files allowed to pass SQL text to the
// database, with the reason sqlc cannot hold it.
var handwrittenSQL = map[string]string{
	"maintenance/partitions.go": "partition DDL names tables, which cannot be parameters; savepoints are transaction control",
	"maintenance/retention.go":  "trims a default partition named at run time",
	"migrations/status.go":      "migration status must work before the application schema exists; locks belong to the migration connection",
}

var sqlMethods = map[string]bool{"Exec": true, "Query": true, "QueryRow": true, "Queue": true}

// TestStorageSQLLivesInQueryFiles keeps SQL in backend/db/queries, where sqlc
// checks it against the schema. A call to Exec, Query, QueryRow or Queue whose
// arguments contain a string literal is SQL written in Go.
func TestStorageSQLLivesInQueryFiles(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	storageRoot := filepath.Join(filepath.Dir(filepath.Dir(currentFile)), "storage", "postgres")
	walkError := filepath.WalkDir(storageRoot, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() && entry.Name() == "sqlc" {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, relativeError := filepath.Rel(storageRoot, path)
		if relativeError != nil {
			return relativeError
		}
		relative = filepath.ToSlash(relative)
		if _, allowed := handwrittenSQL[relative]; allowed {
			return nil
		}
		file, parseError := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseError != nil {
			return parseError
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || !sqlMethods[selector.Sel.Name] {
				return true
			}
			for _, argument := range call.Args {
				if containsStringLiteral(argument) {
					t.Errorf("%s passes SQL text to %s; move it to backend/db/queries", relative, selector.Sel.Name)
					return false
				}
			}
			return true
		})
		return nil
	})
	if walkError != nil {
		t.Fatal(walkError)
	}
}

func containsStringLiteral(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if literal, isLiteral := node.(*ast.BasicLit); isLiteral && literal.Kind == token.STRING {
			found = true
		}
		return !found
	})
	return found
}
