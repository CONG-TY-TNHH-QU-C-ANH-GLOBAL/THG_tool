// Test infra binding for the prompts subpackage. Per
// [[feedback_storetest_scaling_pattern]] each subpackage carries a
// 5-line storetest binding; the template-copy lives in
// internal/store/storetest/.
package prompts_test

import (
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/prompts"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newPromptsStore(t *testing.T, name string) (*store.Store, *prompts.Store) {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, db.Prompts()
}
