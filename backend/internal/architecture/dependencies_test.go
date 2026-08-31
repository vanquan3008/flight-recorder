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
	assertNoImport(t, filepath.Join(internalRoot, "transport"), "/internal/storage/postgres")
	assertNoImport(t, filepath.Join(internalRoot, "ingest"), "/internal/storage/postgres")
	assertNoImport(t, filepath.Join(internalRoot, "storage", "postgres"), "/internal/replay")
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
			if strings.HasSuffix(importPath, forbiddenSuffix) {
				t.Errorf("%s imports forbidden adapter %s", path, importPath)
			}
		}
		return nil
	})
	if walkError != nil {
		t.Fatal(walkError)
	}
}
