// Package storetest is the shared test-infrastructure package for
// internal/store and its subpackages (knowledge, crawl, outbound,
// and future Phase N extractions).
//
// # Why this package exists
//
// Subpackage tests live in `package <subpkg>_test` (external test
// package). External packages cannot reach unexported helpers in
// their parent's test files. Without storetest, each subpackage would
// duplicate the schema-template bootstrap. STORE_SUBPACKAGE_REFACTOR
// rejects that — single source of truth for test infra must survive
// every Phase N extraction without rewrites.
//
// # Why bootstrap-injection (instead of importing store directly)
//
// storetest deliberately does NOT import internal/store. Doing so
// would create a test cycle: store/*_test.go would import storetest,
// storetest would import store, and Go's test compilation pass would
// reject the resulting loop (the test binary links the production
// store package + the test files together; both legs touching
// storetest closes the loop).
//
// Bootstrap-injection breaks the cycle: callers pass a `func(path
// string) error` that opens + migrates a DB. storetest stores the
// compiled template, callers reuse store.New() to open copies. The
// caller's binding is 5 lines per test binary — fixed-size, does not
// grow with the test count. The template-copy implementation lives
// here exactly once.
//
// Idiomatic Go precedent: net/http/httptest, testing/iotest,
// testing/fstest. Each is a regular package consumed by tests across
// many other packages. None of them import the package-under-test.
//
// # Scaling rule for Phase N extractions
//
// When Phase 5 extracts coordination/, Phase 6 extracts identities/,
// etc., each subpackage adds a 5-line helper in its own *_test.go:
//
//	func newCoordinationStore(t *testing.T, name string) *coordination.Store {
//	    dst := storetest.CopyTemplate(t, bootstrapStore, name)
//	    db, _ := store.New(dst)
//	    t.Cleanup(func() { _ = db.Close() })
//	    return db.Coordination()
//	}
//
// No storetest changes per phase. No duplicated schema-bootstrap. The
// shared template's sync.Once is process-scoped, so one migrate cost
// per test binary regardless of caller count.
package storetest

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Bootstrap opens a SQLite file at dbPath, runs migrations, and
// closes the handle. storetest invokes it exactly once per test
// binary via sync.Once to compile the schema template.
//
// All callers in a given test binary MUST use an equivalent
// bootstrap (same migration entrypoint, same final schema). storetest
// memoizes the first invocation; subsequent callers reuse the
// resulting template regardless of which Bootstrap they passed. In
// our codebase every caller funnels through store.New(), so this
// is a non-issue — flagged here for future readers who might add a
// second migration entrypoint.
type Bootstrap func(dbPath string) error

// Shared schema template for ALL store-test binaries.
//
// Why: migrate() runs ~150 idempotent DDL statements. Under the race
// detector + modernc.org/sqlite each Exec is ~5–10ms because the
// libc-emulated pthread primitives serialise heavily. The original
// internal/store package has ~110 tests; running migrate() per test
// burned the full 120s CI budget and hung the runner (panic: test
// timed out after 2m0s inside _pthreadMutexEnter during a fresh
// migrate).
//
// Fix: compile ONE migrated SQLite file at first call, then copy it
// into each test's TempDir. The host store.migrate() detects the
// schema and short-circuits via schemaAlreadyApplied. The cumulative
// migrate cost drops from O(N tests × 150 DDLs) to O(1 × 150 DDLs +
// N file copies).
//
// The template lives in os.TempDir (NOT t.TempDir of the first
// caller) because t.TempDir is scoped to that one test — subsequent
// tests would lose access when the first test ends. We rely on the
// OS to clean up the template directory; on Linux CI the runner
// wipes /tmp at job end.
var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// TemplatePath returns the path to a SQLite file with the full store
// schema already migrated. Safe for concurrent callers; the migrate
// runs exactly once per test binary.
//
// The bootstrap function is invoked only on the first call within a
// given test binary. Later callers in the same binary reuse the
// compiled template regardless of what bootstrap they pass.
func TemplatePath(t *testing.T, bootstrap Bootstrap) string {
	t.Helper()
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "store-schema-template-*")
		if err != nil {
			templateErr = err
			return
		}
		path := filepath.Join(dir, "template.db")
		if err := bootstrap(path); err != nil {
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

// CopyTemplate seeds t.TempDir/<name> from the shared schema template
// and returns the destination path. The caller opens the resulting
// file with store.New() (or the equivalent for their dialect) and
// receives a fully-migrated DB without paying the per-test migrate
// cost.
//
// Returns the destination path so the caller can store.New(dst).
// Cleanup of the t.TempDir is handled by testing.T; storetest does
// not register additional cleanup.
//
// `name` is the desired filename inside t.TempDir (e.g. "knowledge.db").
// Tests should keep using unique names per call so failure
// diagnostics reference a sensible path.
func CopyTemplate(t *testing.T, bootstrap Bootstrap, name string) string {
	t.Helper()
	src := TemplatePath(t, bootstrap)
	dst := filepath.Join(t.TempDir(), name)
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy schema template to %s: %v", dst, err)
	}
	return dst
}

func copyFile(src, dst string) error {
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
