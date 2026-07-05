// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Characterization tests for the two-layer bootstrap topology
// (2026-07-05, bootstrap topology PR1):
//
//   layer 1: versioned migrations (migrator.go + migrations/*.up.sql)
//            own the platform-plane schema — run-once, atomic;
//   layer 2: the local-runtime bootstrap (sessions.Migrate, app.Migrate,
//            run by initDomains) owns the local-runtime-plane tables —
//            idempotent, every boot.
//
// See internal/store/migrations/README.md "Bootstrap layers" and
// docs/architecture/DATABASE_OWNERSHIP.md §Data planes.

// localRuntimeTables are created ONLY by the layer-2 bootstrap — they are
// deliberately absent from the versioned baseline.
var localRuntimeTables = []string{
	"browser_sessions", "app_tasks", "task_leads", "browser_identities",
	"port_registry", "account_rate_limits", "circuit_breaker_state",
	"session_audit_log", "post_seen_cache",
}

// TestBootstrap_DoubleBootIdempotent pins that opening the SAME database
// twice is safe: layer 1 is run-once (schema_migrations) and layer 2 is
// idempotent, so a second store.New must succeed with no error and all
// local-runtime tables present.
func TestBootstrap_DoubleBootIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boot.db")

	first, err := New(path)
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first boot: %v", err)
	}

	second, err := New(path)
	if err != nil {
		t.Fatalf("second boot on same file must be idempotent: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	ctx := context.Background()
	for _, table := range localRuntimeTables {
		if !second.tableExists(ctx, table) {
			t.Errorf("local-runtime table %q missing after double boot", table)
		}
	}
	// The sessions-domain ALTERs must have applied (pins that removing the
	// duplicate block from app.Migrate did not lose the columns).
	if _, err := second.db.Exec(`SELECT heartbeat_at, checkpoint_url FROM browser_sessions LIMIT 1`); err != nil {
		t.Fatalf("browser_sessions ALTER columns missing: %v", err)
	}
	// Layer 1 recorded a schema version (baseline or applied migrations).
	if v, err := second.CurrentSchemaVersion(ctx); err != nil || v < 1 {
		t.Fatalf("schema version after boot = %d, err = %v; want >= 1", v, err)
	}
}

// sanctionedBootstrapFiles are the ONLY production Go files allowed to
// contain CREATE TABLE statements. Everything else must go through the
// versioned migrations directory. Adding a file here requires an
// architecture decision (data-plane classification), not convenience —
// see docs/architecture/DATABASE_OWNERSHIP.md §Data planes.
var sanctionedBootstrapFiles = map[string]bool{
	"internal/store/migrator.go":         true, // schema_migrations registry itself
	"internal/store/sessions/migrate.go": true, // local-runtime plane: browser_sessions
	"internal/store/app/migrate.go":      true, // local-runtime plane: app/browser-infra tables
	"internal/jobs/store.go":             true, // local-runtime plane: scheduler_jobs
}

// TestNoHiddenCreateTableBootstrap fails when a production .go file under
// internal/ gains a CREATE TABLE outside the sanctioned bootstrap layer.
// This is the guard against new hidden bootstraps and in-code schema dumps.
func TestNoHiddenCreateTableBootstrap(t *testing.T) {
	root := filepath.Join("..", "..") // package dir internal/store -> repo root
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.Contains(string(body), "CREATE TABLE") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if !sanctionedBootstrapFiles[filepath.ToSlash(rel)] {
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("CREATE TABLE outside the sanctioned bootstrap layer (add a versioned migration in internal/store/migrations/ instead):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
