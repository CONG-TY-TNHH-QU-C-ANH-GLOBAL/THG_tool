// Package reeltest is the shared Postgres-gated bootstrap for reel-domain
// tests. Both internal/store/reel and internal/services/reel need the same
// "open a real Postgres store, skip if unavailable, clean up on exit" setup
// — external test packages (package <pkg>_test) can't share unexported
// helpers across directories, so this mirrors internal/store/storetest's
// bootstrap-injection precedent: a small standalone package, not a
// production dependency.
package reeltest

import (
	"context"
	"os"
	"testing"

	"github.com/thg/scraper/internal/store"
)

// OpenStore opens a real Postgres-backed store for reel-domain tests,
// gated on POSTGRES_PLATFORM_TEST_DSN (skips the test if unset — reel has
// no SQLite schema, see
// docs/architecture/decisions/ADR-reel-studio-platform-module.md). Registers
// cleanup that closes the connection. It does NOT delete any rows — this is
// a real, possibly-shared Postgres database (see CleanupOrgs).
func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping reel Postgres tests")
	}
	s, err := store.New(dsn)
	if err != nil {
		t.Fatalf("store.New(postgres dsn): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// CleanupOrgs registers best-effort, org-scoped teardown for every org a
// test created rows under. Deletes ONLY the given org IDs' rows — never a
// table-wide DELETE, since POSTGRES_PLATFORM_TEST_DSN can point at a
// database other tests/processes share.
func CleanupOrgs(t *testing.T, s *store.Store, orgIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		// Best-effort teardown: a failure here would only leak this test's
		// rows into the next run (harmless — every test uses org IDs
		// scoped to itself), never mask a real assertion.
		ctx := context.Background()
		for _, orgID := range orgIDs {
			_, _ = s.DB().ExecContext(ctx, `DELETE FROM reel_scripts WHERE org_id = $1`, orgID)
			_, _ = s.DB().ExecContext(ctx, `DELETE FROM reels WHERE org_id = $1`, orgID)
		}
	})
}
