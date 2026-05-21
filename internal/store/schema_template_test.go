// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"testing"

	"github.com/thg/scraper/internal/store/storetest"
)

// bootstrapStore is the storetest Bootstrap binding for this test
// binary: opens a fresh SQLite file, runs migrations via store.New(),
// and closes the handle so the file becomes the schema template.
//
// Per-binary binding pattern: see internal/store/storetest/storetest.go
// package doc. Every subpackage test binary defines its own three-line
// equivalent. The actual template-copy machinery is single-sourced in
// storetest.
func bootstrapStore(path string) error {
	db, err := New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// newSharedStore returns a fresh *Store seeded with the migrated
// schema template. The DB file lives in t.TempDir so it is
// auto-cleaned with the test. 100x faster than running migrate() per
// test under the race detector (see storetest doc for the pthread
// rationale).
//
// Kept under this name as a convenience for the 15+ existing test
// files that already call newSharedStore. New callers in subpackage
// tests (package knowledge_test, future coordination_test, …) build
// their own thin wrappers in their own *_test.go — see
// internal/store/knowledge/testing_helpers_test.go for the canonical
// shape.
func newSharedStore(t *testing.T, name string) *Store {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
