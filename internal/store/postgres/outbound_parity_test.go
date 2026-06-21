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

	planned := readExpect(t, h, org, models.ExecPlanned, "", 10, 3)
	if !isCreatedAtDesc(planned) {
		t.Fatalf("results must be ordered created_at DESC")
	}

	for _, m := range readExpect(t, h, org, models.ExecPlanned, "comment", 10, 2) {
		assertUnsentComment(t, m, org)
	}

	readExpect(t, h, org, models.ExecExecuting, "", 10, 0) // state filter
	readExpect(t, h, org, models.ExecPlanned, "", 2, 2)    // limit
}

// assertUnsentComment checks one freshly-read planned comment row scanned its
// key fields correctly and carries no sent_at.
func assertUnsentComment(t *testing.T, m models.OutboundMessage, org int64) {
	t.Helper()
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
