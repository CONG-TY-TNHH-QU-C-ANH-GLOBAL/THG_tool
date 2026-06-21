package postgres_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// runOutboundLifecycleParitySuite asserts the SAME outbound lifecycle contract
// against any backend harness. Behavior assertions are shared; only seed/setup
// differs per backend (see parity_harness_test.go).
//
// Scope: the suite asserts the OutboundLifecycleRepository SEAM contract
// (outbound_messages state machine + ClaimResult + read model). It does NOT
// assert execution_attempts rows — that audit append is coordination-owned and
// wired via a composition-root hook only in the SQLite path (topology gate §5 /
// package doc); it is the single documented backend difference.
//
// Concurrency: single-winner is verified by SEQUENTIAL double-claim, never
// goroutine storms or t.Parallel(), so SQLite is never pushed into physical
// lock limits.
func runOutboundLifecycleParitySuite(t *testing.T, h parityHarness) {
	t.Run("read_filters", func(t *testing.T) { parityReadFilters(t, h) })
	t.Run("claim_single_winner", func(t *testing.T) { parityClaim(t, h) })
	t.Run("finalize_cas_replay", func(t *testing.T) { parityFinalize(t, h) })
	t.Run("reset_stale", func(t *testing.T) { parityResetStale(t, h) })
	t.Run("org_isolation", func(t *testing.T) { parityOrgIsolation(t, h) })
}

// TestSQLiteOutboundLifecycleParity runs the shared suite against the active
// SQLite store. TestPostgresOutboundLifecycleParity runs the identical suite
// against the Postgres adapter (skips without POSTGRES_TEST_DSN).
func TestSQLiteOutboundLifecycleParity(t *testing.T) {
	runOutboundLifecycleParitySuite(t, setupSQLiteHarness(t))
}

func TestPostgresOutboundLifecycleParity(t *testing.T) {
	runOutboundLifecycleParitySuite(t, setupPostgresHarness(t))
}

func parityReadFilters(t *testing.T, h parityHarness) {
	const org = int64(1101)
	h.seedPlanned(t, org, 11, "comment", "https://fb.com/r1")
	h.seedPlanned(t, org, 11, "comment", "https://fb.com/r2")
	h.seedPlanned(t, org, 11, "inbox", "https://fb.com/r3")

	planned, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "", 10)
	if err != nil {
		t.Fatalf("read planned: %v", err)
	}
	if len(planned) != 3 {
		t.Fatalf("want 3 planned, got %d", len(planned))
	}
	if !isCreatedAtDesc(planned) {
		t.Fatalf("results must be ordered created_at DESC")
	}

	comments, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "comment", 10)
	if err != nil {
		t.Fatalf("read comment: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("want 2 comment, got %d", len(comments))
	}
	for _, m := range comments {
		if m.Type != "comment" || m.OrgID != org {
			t.Fatalf("type/org filter wrong: %+v", m)
		}
		if m.CreatedAt.IsZero() {
			t.Fatalf("created_at must scan non-zero")
		}
		if !m.SentAt.IsZero() {
			t.Fatalf("sent_at must be zero for an unsent row, got %v", m.SentAt)
		}
	}

	if ex, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecExecuting, "", 10); err != nil || len(ex) != 0 {
		t.Fatalf("want 0 executing, got %d err=%v", len(ex), err)
	}
	if lim, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "", 2); err != nil || len(lim) != 2 {
		t.Fatalf("limit 2 want 2, got %d err=%v", len(lim), err)
	}
}

func parityClaim(t *testing.T, h parityHarness) {
	const org = int64(1102)
	id := h.seedPlanned(t, org, 11, "comment", "https://fb.com/c1")

	before := time.Now()
	const lease = 2 * time.Minute
	claim, err := h.repo.ClaimPlannedOutboundForOrg(org, id, "worker-a", lease)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claim == nil || claim.ExecutionID == "" {
		t.Fatalf("claim must return a ClaimResult with an execution id")
	}
	if !claim.LeaseExpiry.After(before) {
		t.Fatalf("lease_expiry must be after claim start")
	}
	// Precision-safe: lease ~= claim time + leaseDuration, generous tolerance.
	assertWithin(t, "lease_expiry", before.Add(lease), claim.LeaseExpiry, 30*time.Second)

	// Sequential double-claim → single winner (no goroutine storm). Both
	// backends return sql.ErrNoRows for a row that is no longer planned.
	if _, err := h.repo.ClaimPlannedOutboundForOrg(org, id, "worker-b", lease); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second claim must return sql.ErrNoRows, got %v", err)
	}

	ex, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecExecuting, "", 10)
	if err != nil {
		t.Fatalf("read executing: %v", err)
	}
	if len(ex) != 1 || ex[0].ExecutionID != claim.ExecutionID || ex[0].ExecutionState != models.ExecExecuting {
		t.Fatalf("claimed row wrong: %+v", ex)
	}
}
