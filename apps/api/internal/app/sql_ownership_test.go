package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

var staticSQLPattern = regexp.MustCompile(
	"(?is)(?:\"|`)\\s*(?:SELECT\\s+[a-z_*]|INSERT\\s+INTO\\s+[a-z]|" +
		"UPDATE\\s+[a-z][a-z0-9_]*\\s+SET|DELETE\\s+FROM\\s+[a-z]|WITH\\s+[a-z])",
)

type sqlOwnershipFinding struct {
	path  string
	match string
}

func TestStaticProductionSQLIsOwnedBySQLC(t *testing.T) {
	allowed := map[string][]string{
		"app/store/store.go": {
			`"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"`,
		},
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve the SQL ownership test location")
	}
	internalRoot := filepath.Dir(filepath.Dir(filename))
	findings, err := findStaticProductionSQL(os.DirFS(internalRoot), allowed)
	if err != nil {
		t.Fatalf("scan production SQL ownership: %v", err)
	}
	for _, finding := range findings {
		t.Errorf("static SQL %q remains outside sqlc in %s", finding.match, finding.path)
	}
}

func TestStaticProductionSQLScannerCoversSlicesAndSkipsGeneratedSources(t *testing.T) {
	source := fstest.MapFS{
		"routes/example/service.go": {
			Data: []byte(`package example; const query = "SELECT id FROM jobs"`),
		},
		"routes/example/service_test.go": {
			Data: []byte(`package example; const query = "SELECT id FROM jobs"`),
		},
		"routes/example/gen/queries.sql.go": {
			Data: []byte(`package gen; const query = "SELECT id FROM jobs"`),
		},
		"app/store/store.go": {
			Data: []byte(`package store; const query = "SELECT name FROM sqlite_master"`),
		},
	}
	findings, err := findStaticProductionSQL(source, map[string][]string{
		"app/store/store.go": {`"SELECT name FROM sqlite_master"`},
	})
	if err != nil {
		t.Fatalf("scan the fixture: %v", err)
	}
	if len(findings) != 1 || findings[0].path != "routes/example/service.go" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func findStaticProductionSQL(
	sourceFS fs.FS,
	allowed map[string][]string,
) ([]sqlOwnershipFinding, error) {
	findings := make([]sqlOwnershipFinding, 0)
	err := fs.WalkDir(sourceFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "gen" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := fs.ReadFile(sourceFS, path)
		if err != nil {
			return err
		}
		remaining := string(source)
		for _, exception := range allowed[path] {
			remaining = strings.ReplaceAll(remaining, exception, "")
		}
		if match := staticSQLPattern.FindString(remaining); match != "" {
			findings = append(findings, sqlOwnershipFinding{path: path, match: match})
		}
		return nil
	})
	return findings, err
}
