package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const httpapiModule = "github.com/vanquan3008/tracepilot/internal/transport/httpapi"

// TestHTTPFeaturesStayIndependent keeps each HTTP feature to the shared contract: a feature may use
// api, render, and openapi, but not the router, the admission guard, or another feature.
func TestHTTPFeaturesStayIndependent(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	featuresRoot := filepath.Join(filepath.Dir(filepath.Dir(currentFile)), "transport", "httpapi", "features")
	features, readError := os.ReadDir(featuresRoot)
	if readError != nil || len(features) == 0 {
		t.Fatalf("read %s: %v", featuresRoot, readError)
	}
	for _, feature := range features {
		if !feature.IsDir() {
			continue
		}
		own := httpapiModule + "/features/" + feature.Name()
		files, _ := filepath.Glob(filepath.Join(featuresRoot, feature.Name(), "*.go"))
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, parseError := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if parseError != nil {
				t.Fatal(parseError)
			}
			for _, imported := range parsed.Imports {
				importPath, _ := strconv.Unquote(imported.Path.Value)
				if forbiddenFeatureImport(importPath, own) {
					t.Errorf("%s imports %s", file, importPath)
				}
			}
		}
	}
}

func forbiddenFeatureImport(importPath, own string) bool {
	switch {
	case importPath == httpapiModule:
		return true
	case strings.HasPrefix(importPath, httpapiModule+"/access"):
		return true
	case strings.HasPrefix(importPath, httpapiModule+"/features/"):
		return importPath != own && !strings.HasPrefix(importPath, own+"/")
	}
	return false
}

func TestForbiddenFeatureImport(t *testing.T) {
	own := httpapiModule + "/features/traces"
	for _, path := range []string{httpapiModule, httpapiModule + "/access", httpapiModule + "/features/logs"} {
		if !forbiddenFeatureImport(path, own) {
			t.Errorf("guard missed %s", path)
		}
	}
	for _, path := range []string{httpapiModule + "/api", httpapiModule + "/render", own} {
		if forbiddenFeatureImport(path, own) {
			t.Errorf("guard refused %s", path)
		}
	}
}

// TestClickHouseStoresStayIndependent keeps each ClickHouse store to the shared client: a store may
// not import the joined clickhouse.Store, the schema checks, or another store.
func TestClickHouseStoresStayIndependent(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	root := filepath.Join(filepath.Dir(filepath.Dir(currentFile)), "storage", "clickhouse")
	stores := []string{"logs", "traces", "metrics", "analytics", "detection", "rollups", "retention"}
	for _, store := range stores {
		assertNoImport(t, filepath.Join(root, store), "/internal/storage/clickhouse/schema")
		for _, other := range stores {
			if other != store {
				assertNoImport(t, filepath.Join(root, store), "/internal/storage/clickhouse/"+other)
			}
		}
		assertNoImport(t, filepath.Join(root, store), "/internal/storage/hybrid")
	}
	assertNoImport(t, filepath.Join(root, "client"), "/internal/storage/hybrid")
}
