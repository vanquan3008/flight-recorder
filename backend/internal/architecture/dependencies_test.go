package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAdapterDependencies(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	internalRoot := filepath.Dir(filepath.Dir(currentFile))
	for _, adapter := range []string{"config", "storage", "transport", "ingest"} {
		assertNoImport(t, filepath.Join(internalRoot, "domain"), "/internal/"+adapter)
		assertNoImport(t, filepath.Join(internalRoot, "application"), "/internal/"+adapter)
	}
	for _, service := range []string{"application", "auth", "observability", "query"} {
		assertNoImport(t, filepath.Join(internalRoot, "domain"), "/internal/"+service)
	}
	assertNoImport(t, filepath.Join(internalRoot, "transport"), "/internal/storage")
	assertNoImport(t, filepath.Join(internalRoot, "ingest"), "/internal/storage")
	assertNoImport(t, filepath.Join(internalRoot, "storage"), "/internal/application")
}

func assertNoImport(t *testing.T, root, forbiddenSuffix string) {
	t.Helper()
	walkError := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil || entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return walkError
		}
		parsed, parseError := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseError != nil {
			return parseError
		}
		for _, imported := range parsed.Imports {
			importPath, unquoteError := strconv.Unquote(imported.Path.Value)
			if unquoteError != nil {
				return unquoteError
			}
			if forbiddenImport(importPath, forbiddenSuffix) {
				t.Errorf("%s imports forbidden adapter %s", path, importPath)
			}
		}
		return nil
	})
	if walkError != nil {
		t.Fatal(walkError)
	}
}

func forbiddenImport(importPath, suffix string) bool {
	if strings.HasPrefix(suffix, "/internal/") {
		root := "github.com/vanquan3008/tracepilot" + suffix
		return importPath == root || strings.HasPrefix(importPath, root+"/")
	}
	root := strings.TrimPrefix(suffix, "/")
	return importPath == root || strings.HasPrefix(importPath, root+"/")
}

func TestForbiddenImportIncludesSubpackages(t *testing.T) {
	for _, path := range []string{"github.com/vanquan3008/tracepilot/internal/storage", "github.com/vanquan3008/tracepilot/internal/storage/postgres"} {
		if !forbiddenImport(path, "/internal/storage") {
			t.Errorf("guard missed %s", path)
		}
	}
	if forbiddenImport("github.com/vanquan3008/tracepilot/internal/storageextra", "/internal/storage") {
		t.Fatal("guard matched a sibling package")
	}
}
