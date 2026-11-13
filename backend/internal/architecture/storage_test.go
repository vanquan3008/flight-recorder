package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const storageModule = "github.com/vanquan3008/tracepilot/internal/storage/postgres"

// storageFeatureImports lists which sibling packages each storage package may
// use. Every store runs generated sqlc queries; only work that must share
// another store's transaction crosses stores: ingest opens incidents, and
// incidents queue webhook deliveries.
var storageFeatureImports = map[string][]string{
	"pgvalue":     {},
	"pgbatch":     {},
	"outbox":      {"pgvalue", "sqlc"},
	"incidents":   {"pgvalue", "pgbatch", "outbox", "sqlc"},
	"signals":     {"pgvalue", "pgbatch", "incidents", "sqlc"},
	"analytics":   {"pgvalue", "incidents", "outbox", "sqlc"},
	"explorer":    {"pgvalue", "sqlc"},
	"identity":    {"pgvalue", "sqlc"},
	"webhooks":    {"pgvalue", "sqlc"},
	"replays":     {"pgvalue", "sqlc"},
	"maintenance": {"sqlc"},
	"migrations":  {},
}

func TestStorageFeatureDependencies(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	storageRoot := filepath.Join(filepath.Dir(filepath.Dir(currentFile)), "storage", "postgres")
	entries, readError := os.ReadDir(storageRoot)
	if readError != nil {
		t.Fatal(readError)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "sqlc" {
			continue
		}
		allowed, known := storageFeatureImports[entry.Name()]
		if !known {
			t.Errorf("storage package %s has no entry in storageFeatureImports", entry.Name())
			continue
		}
		for _, imported := range storageImports(t, filepath.Join(storageRoot, entry.Name())) {
			if !slices.Contains(allowed, imported) {
				t.Errorf("storage/postgres/%s must not import %s", entry.Name(), imported)
			}
		}
	}
}

// storageImports returns the storage/postgres packages a directory imports,
// relative to storage/postgres; the composition root itself is reported as ".".
func storageImports(t *testing.T, directory string) []string {
	t.Helper()
	files, globError := filepath.Glob(filepath.Join(directory, "*.go"))
	if globError != nil {
		t.Fatal(globError)
	}
	var imported []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, parseError := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseError != nil {
			t.Fatal(parseError)
		}
		for _, spec := range parsed.Imports {
			importPath, unquoteError := strconv.Unquote(spec.Path.Value)
			if unquoteError != nil {
				t.Fatal(unquoteError)
			}
			if importPath == storageModule {
				imported = append(imported, ".")
			} else if relative, inside := strings.CutPrefix(importPath, storageModule+"/"); inside {
				imported = append(imported, relative)
			}
		}
	}
	return imported
}
