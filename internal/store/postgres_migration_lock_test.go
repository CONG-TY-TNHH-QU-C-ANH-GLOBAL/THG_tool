// Domain: infra (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// TestPostgresMigrationAdvisoryLock_SerializesBoot proves runMigrations'
// Postgres path actually blocks a concurrent boot instead of racing it: it
// takes the SAME session-advisory-lock key store.New uses on a separate raw
// connection (simulating another instance mid-migration), starts New(dsn) in
// a goroutine, and asserts New has NOT returned while the lock is held, then
// that it proceeds promptly once the lock is released.
//
// Skipped (not failed) unless POSTGRES_PLATFORM_TEST_DSN is set — same gate
// as TestRealPostgresApply.
func TestPostgresMigrationAdvisoryLock_SerializesBoot(t *testing.T) {
	dsn := os.Getenv("POSTGRES_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_PLATFORM_TEST_DSN not set; skipping real PostgreSQL advisory-lock validation")
	}
	ctx := context.Background()

	holderDB, err := sql.Open(dbutil.PostgresDriverName, dsn)
	if err != nil {
		t.Fatalf("open holder db: %v", err)
	}
	defer holderDB.Close()
	holder, err := holderDB.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire holder conn: %v", err)
	}
	defer holder.Close()

	if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockKey); err != nil {
		t.Fatalf("holder acquire advisory lock: %v", err)
	}

	type bootResult struct {
		store *Store
		err   error
	}
	done := make(chan bootResult, 1)
	go func() {
		s, err := New(dsn)
		done <- bootResult{store: s, err: err}
	}()

	select {
	case r := <-done:
		if r.store != nil {
			_ = r.store.Close()
		}
		t.Fatalf("store.New returned (err=%v) while the migration advisory lock was held by another session — boot is not serialized", r.err)
	case <-time.After(1 * time.Second):
		// Expected: New is still blocked on pg_advisory_lock.
	}

	if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockKey); err != nil {
		t.Fatalf("holder release advisory lock: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("store.New after lock release: %v", r.err)
		}
		_ = r.store.Close()
	case <-time.After(10 * time.Second):
		t.Fatal("store.New did not proceed after the advisory lock was released")
	}
}
