// Domain: coordination test infra.
//
// Per [[feedback_storetest_scaling_pattern]] every subpackage extracts
// the schema-bootstrap binding into a 5-line helper in its own *_test.go.
// The template-copy machinery lives in `internal/store/storetest/` exactly
// once.
package coordination_test

import (
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/storetest"
)

// bootstrapStore opens a fresh SQLite file, runs migrations via
// store.New(), and closes the handle. storetest invokes this exactly
// once per test binary via sync.Once to compile the schema template.
func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// newCoordinationStore returns a fresh full store + its coordination
// handle, seeded from the migrated schema template. Test cleanup is
// registered via t.Cleanup.
//
// Two-handle return because some tests need to plant rows in peer
// domains (outbound_messages) directly via *store.Store.DB() while
// asserting coordination state via the typed coordination handle.
func newCoordinationStore(t *testing.T, name string) (*store.Store, *coordination.Store) {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, db.Coordination()
}
