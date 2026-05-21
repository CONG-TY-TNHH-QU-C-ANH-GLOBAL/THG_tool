// testing_helpers_test.go is the per-binary binding to the shared
// storetest infrastructure for this subpackage's integration tests.
//
// Pattern is fixed-size: every subpackage extracted from internal/store
// (knowledge today, coordination / identities / leads tomorrow) ships
// the same ~5-line bootstrap + ~5-line accessor in its own test
// directory. The actual template-copy machinery is single-sourced in
// internal/store/storetest. See that package's doc for the cycle-
// avoidance rationale.
package knowledge_test

import (
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/knowledge"
	"github.com/thg/scraper/internal/store/storetest"
)

// bootstrapStore is the storetest Bootstrap binding for this test
// binary: opens a fresh SQLite file via store.New() so migrations
// run, then closes it. storetest invokes this exactly once per test
// binary (sync.Once) to compile the schema template.
func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// newKnowledgeStore returns a fresh *knowledge.Store seeded with the
// migrated schema template. Knowledge tests in this package call this
// instead of opening a DB directly — they get a fully-migrated catalog
// without paying the per-test migrate cost.
func newKnowledgeStore(t *testing.T, name string) *knowledge.Store {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.Knowledge()
}
