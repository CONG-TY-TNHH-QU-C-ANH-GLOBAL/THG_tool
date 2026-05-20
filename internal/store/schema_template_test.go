// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Shared schema template for the `internal/store` test package.
//
// Why: migrate() runs ~150 idempotent DDL statements. Under the race
// detector + modernc.org/sqlite each Exec is ~5–10ms because the
// libc-emulated pthread primitives serialise heavily. The `internal/store`
// package has ~110 tests; running migrate() per test burned the full
// 120s CI budget and hung the runner (panic: test timed out after 2m0s
// inside _pthreadMutexEnter during a fresh migrate).
//
// Fix: build ONE migrated SQLite file at first call, then copy it into
// each test's TempDir. store.migrate() detects the schema and
// short-circuits via schemaAlreadyApplied. The cumulative migrate cost
// drops from O(N tests × 150 DDLs) to O(1 × 150 DDLs + N file copies).
//
// The template lives in os.TempDir (NOT t.TempDir of the first caller)
// because t.TempDir is scoped to that one test — subsequent tests would
// lose access when the first test ends. We rely on the OS to clean up
// the template directory; on Linux CI the runner wipes /tmp at job
// end.
var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// schemaTemplatePath returns the path to a SQLite file with the full
// store schema already migrated. Safe for concurrent callers; the
// migrate runs exactly once.
func schemaTemplatePath(t *testing.T) string {
	t.Helper()
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "store-schema-template-*")
		if err != nil {
			templateErr = err
			return
		}
		path := filepath.Join(dir, "template.db")
		db, err := New(path)
		if err != nil {
			templateErr = err
			return
		}
		if err := db.Close(); err != nil {
			templateErr = err
			return
		}
		templatePath = path
	})
	if templateErr != nil {
		t.Fatalf("schema template: %v", templateErr)
	}
	return templatePath
}

// newSharedStore returns a fresh *Store seeded with the package schema
// template. The DB file lives in t.TempDir so it is auto-cleaned with
// the test. Use this instead of `New(filepath.Join(t.TempDir(), ...))`
// in test helpers — same behaviour, 100x faster under -race.
//
// `name` is the desired filename inside t.TempDir (e.g. "knowledge.db").
// Tests should keep using unique names per call so failure diagnostics
// reference a sensible path.
func newSharedStore(t *testing.T, name string) *Store {
	t.Helper()
	src := schemaTemplatePath(t)
	dst := filepath.Join(t.TempDir(), name)
	if err := copyTemplateFile(src, dst); err != nil {
		t.Fatalf("copy schema template to %s: %v", dst, err)
	}
	db, err := New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func copyTemplateFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()
	if _, err := io.Copy(df, sf); err != nil {
		return err
	}
	return df.Sync()
}
