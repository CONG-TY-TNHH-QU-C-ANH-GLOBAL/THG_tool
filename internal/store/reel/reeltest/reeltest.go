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
// docs/architecture/decisions/ADR-reel-studio-platform-module.md).
// Registers cleanup that clears reel_scripts/reels and closes the
// connection.
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
	t.Cleanup(func() {
		// Best-effort teardown: a failure here would only leak test rows
		// into the next run (harmless — every test uses org IDs scoped to
		// its own test), never mask a real assertion.
		ctx := context.Background()
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reel_scripts`)
		_, _ = s.DB().ExecContext(ctx, `DELETE FROM reels`)
		_ = s.Close()
	})
	return s
}
