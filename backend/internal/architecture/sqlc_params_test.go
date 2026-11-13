package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// autoNamedField matches the names sqlc invents for a parameter it could not
// name: Column1, or a name built from an expression such as
// FromTimetimestamptz1HourpgCataloginterval.
var autoNamedField = regexp.MustCompile(`^Column\d+$|pgCatalog`)

// TestSqlcParametersAreNamed fails when a query parameter has a generated name.
// That happens when a parameter sits inside an expression, for example
// `@from_time::timestamptz - interval '1 hour'`. sqlc then adds an extra field the
// caller never sets, so the bound it was meant to apply silently becomes the
// zero value. Name the parameter (`@parent_window_start::timestamptz`) and compute
// the value in Go.
func TestSqlcParametersAreNamed(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	generated := filepath.Join(filepath.Dir(filepath.Dir(currentFile)), "storage", "postgres", "sqlc")
	walkError := filepath.WalkDir(generated, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return walkError
		}
		file, parseError := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseError != nil {
			return parseError
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, isType := node.(*ast.TypeSpec)
			if !isType || !strings.HasSuffix(spec.Name.Name, "Params") {
				return true
			}
			structType, isStruct := spec.Type.(*ast.StructType)
			if !isStruct {
				return true
			}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if autoNamedField.MatchString(name.Name) {
						t.Errorf("%s.%s has a generated name; name the parameter in the query (see this test)", spec.Name.Name, name.Name)
					}
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
