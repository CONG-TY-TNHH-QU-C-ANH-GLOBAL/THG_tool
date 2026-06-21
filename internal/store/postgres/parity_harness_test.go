package postgres_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/outbound"
)

// outboundLifecycle is the test-local view of the PR10 seam
// (internal/server/agent.OutboundLifecycleRepository). Defined here so the
// shared parity suite runs against both backends without importing the agent
// package's heavy transitive deps — both *store.Store and
// *postgres.OutboundStore already satisfy the real seam (asserted in
// outbound_test.go and internal/server/agent).
type outboundLifecycle interface {
	GetOutboundByExecutionStateForOrg(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error)
	ClaimPlannedOutboundForOrg(orgID, id int64, workerID string, leaseDuration time.Duration) (*outbound.ClaimResult, error)
	FinalizeOutboundAttempt(ctx context.Context, orgID, id int64, executionID string, terminalState models.ExecutionState, verificationOutcome models.VerificationOutcome) (bool, models.ExecutionState, models.VerificationOutcome, string, error)
	ResetStaleExecutingForOrg(orgID int64, staleAfter time.Duration) error
}

// parityHarness wires one backend into the shared lifecycle suite. Only the
// seed/setup helpers differ per backend; the behavior assertions are shared.
type parityHarness struct {
	name        string
	repo        outboundLifecycle
	seedPlanned func(t *testing.T, orgID, accountID int64, msgType, targetURL string) int64
	// makeStale forces a claimed row's lease into the past via backend SQL so
	// reset-stale is deterministic (no flaky sleeps, no timestamp-equality).
	makeStale func(t *testing.T, orgID, id int64)
}

// setupSQLiteHarness builds a harness over a fresh temp SQLite store — the
// active runtime implementation. Seeds via the low-level Insert escape hatch.
func setupSQLiteHarness(t *testing.T) parityHarness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parity.db")
	db, err := store.New(path)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return parityHarness{
		name: "sqlite",
		repo: db,
		seedPlanned: func(t *testing.T, orgID, accountID int64, msgType, targetURL string) int64 {
			t.Helper()
			id, err := db.Outbound().Insert(&models.OutboundMessage{
				OrgID: orgID, Type: msgType, Platform: "facebook", AccountID: accountID,
				TargetURL: targetURL, Content: "hello", ExecutionState: models.ExecPlanned,
			})
			if err != nil {
				t.Fatalf("seed sqlite planned: %v", err)
			}
			return id
		},
		makeStale: func(t *testing.T, orgID, id int64) {
			t.Helper()
			// Short raw connection (WAL + busy_timeout make this safe) to push
			// the lease clearly into the past — avoids SQLite second-precision
			// text-comparison edge cases a tiny-lease+sleep would hit.
			raw, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(15000)")
			if err != nil {
				t.Fatalf("open raw sqlite: %v", err)
			}
			defer raw.Close()
			if _, err := raw.Exec(
				`UPDATE outbound_messages SET lease_expiry = datetime('now','-1 hour') WHERE id = ? AND org_id = ?`,
				id, orgID,
			); err != nil {
				t.Fatalf("sqlite makeStale: %v", err)
			}
		},
	}
}

// setupPostgresHarness builds a harness over the PR11 Postgres adapter. Reuses
// newTestStore (skips when POSTGRES_TEST_DSN is unset; applies the migration +
// cleanup). Seeds via INSERT ... RETURNING id — never LastInsertId.
func setupPostgresHarness(t *testing.T) parityHarness {
	t.Helper()
	adapter, pool := newTestStore(t)
	return parityHarness{
		name: "postgres",
		repo: adapter,
		seedPlanned: func(t *testing.T, orgID, accountID int64, msgType, targetURL string) int64 {
			t.Helper()
			var id int64
			err := pool.QueryRow(context.Background(),
				`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, content, execution_state)
				 VALUES ($1, $2, 'facebook', $3, $4, 'hello', 'planned') RETURNING id`,
				orgID, msgType, accountID, targetURL,
			).Scan(&id)
			if err != nil {
				t.Fatalf("seed postgres planned: %v", err)
			}
			return id
		},
		makeStale: func(t *testing.T, orgID, id int64) {
			t.Helper()
			if _, err := pool.Exec(context.Background(),
				`UPDATE outbound_messages SET lease_expiry = NOW() - INTERVAL '1 hour' WHERE id = $1 AND org_id = $2`,
				id, orgID,
			); err != nil {
				t.Fatalf("postgres makeStale: %v", err)
			}
		},
	}
}

// assertWithin fails unless actual is within tol of expected. Used for all DB
// round-tripped timestamps so SQLite (second/ms text) and Postgres (µs
// timestamptz) precision differences never cause flakes — never exact equality.
func assertWithin(t *testing.T, label string, expected, actual time.Time, tol time.Duration) {
	t.Helper()
	d := actual.Sub(expected)
	if d < 0 {
		d = -d
	}
	if d > tol {
		t.Fatalf("%s: %v not within %v of %v (diff %v)", label, actual, tol, expected, d)
	}
}

// isCreatedAtDesc verifies newest-first ordering with ties allowed — a
// precision-safe check that ORDER BY created_at DESC was applied.
func isCreatedAtDesc(msgs []models.OutboundMessage) bool {
	for i := 1; i < len(msgs); i++ {
		if msgs[i].CreatedAt.After(msgs[i-1].CreatedAt) {
			return false
		}
	}
	return true
}

// readExpect runs a list read and asserts the row count, returning the rows.
// Centralizes the err + count checks so the shared parity assertions stay flat
// (keeps caller cognitive complexity low without weakening any check).
func readExpect(t *testing.T, h parityHarness, org int64, state models.ExecutionState, msgType string, limit, want int) []models.OutboundMessage {
	t.Helper()
	rows, err := h.repo.GetOutboundByExecutionStateForOrg(org, state, msgType, limit)
	if err != nil {
		t.Fatalf("read %s/%q: %v", state, msgType, err)
	}
	if len(rows) != want {
		t.Fatalf("read %s/%q: want %d rows, got %d", state, msgType, want, len(rows))
	}
	return rows
}
