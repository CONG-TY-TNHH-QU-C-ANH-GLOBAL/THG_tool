// Domain: coordination (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// ReconcileEngagement repairs historical false-positive "touched"
// states. These tests pin the rules so a future change cannot
// silently re-introduce the May-2026 incident.

func newReconcileTestStore(t *testing.T) *Store {
	return newSharedStore(t, "reconcile_engagement.db")
}

// Helper: seed a ledger row directly (bypassing QueueOutboundForOrg
// so we control outcome). Returns the inserted id.
func seedLedgerRow(t *testing.T, db *Store, orgID, outboundID int64, outcome, target string) int64 {
	t.Helper()
	id, err := db.RecordActionLedger(context.Background(), ActionLedgerEntry{
		OrgID:       orgID,
		ActionType:  "comment",
		TargetType:  "post",
		TargetURL:   target,
		AccountID:   1,
		OutboundID:  outboundID,
		PerformedAt: time.Now().UTC(),
		Outcome:     outcome,
		Reason:      "",
	})
	if err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	return id
}

// Helper: seed an execution_attempts row with a specific outcome.
func seedAttemptRow(t *testing.T, db *Store, orgID, outboundID int64, outcome models.ExecutionOutcome, target string) int64 {
	t.Helper()
	id, err := db.BeginExecutionAttempt(context.Background(), models.ExecutionAttempt{
		OrgID:      orgID,
		OutboundID: outboundID,
		AccountID:  1,
		TargetURL:  target,
		ActionType: "comment",
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if err := db.FinishExecutionAttempt(context.Background(), id, outcome, string(outcome), VerificationEvidence{}); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}
	return id
}

func TestReconcileEngagement_FlipsBadSucceeded(t *testing.T) {
	db := newReconcileTestStore(t)
	ctx := context.Background()
	target := "https://facebook.com/p/recon-bad"

	// INCIDENT shape: ledger says succeeded, but the execution_attempts
	// row says redirected_feed (wrong post). This is what the May-2026
	// route-mismatch incident left in the DB before
	// EnforceTargetIdentity existed.
	seedLedgerRow(t, db, 1, 100, LedgerOutcomeSucceeded, target)
	seedAttemptRow(t, db, 1, 100, models.ExecutionRedirectedFeed, target)

	report, err := db.ReconcileEngagement(ctx, 1)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.LedgerCorrected != 1 {
		t.Fatalf("expected 1 corrected ledger row, got %+v", report)
	}

	// The ledger row now reads outcome='failed' with reason containing
	// the actual attempt outcome.
	entries, err := db.ListActionLedger(ctx, 1, "", target, time.Time{}, 10)
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Outcome != LedgerOutcomeFailed {
		t.Fatalf("ledger outcome should be failed after reconcile, got %q", entries[0].Outcome)
	}
	if entries[0].Reason == "" || entries[0].Reason[:len("reconciled:")] != "reconciled:" {
		t.Fatalf("ledger reason should be 'reconciled:...', got %q", entries[0].Reason)
	}
}

func TestReconcileEngagement_KeepsGoodSucceeded(t *testing.T) {
	db := newReconcileTestStore(t)
	ctx := context.Background()
	target := "https://facebook.com/p/recon-good"

	// Ledger says succeeded, attempts confirms it. Reconcile must
	// NOT touch this row.
	seedLedgerRow(t, db, 2, 200, LedgerOutcomeSucceeded, target)
	seedAttemptRow(t, db, 2, 200, models.ExecutionDOMVerified, target)

	report, err := db.ReconcileEngagement(ctx, 2)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.LedgerCorrected != 0 || report.LedgerKept != 1 {
		t.Fatalf("expected 0 corrected, 1 kept; got %+v", report)
	}
	entries, _ := db.ListActionLedger(ctx, 2, "", target, time.Time{}, 10)
	if entries[0].Outcome != LedgerOutcomeSucceeded {
		t.Fatalf("good row must NOT be downgraded; got %q", entries[0].Outcome)
	}
}

func TestReconcileEngagement_LeavesOrphansAlone(t *testing.T) {
	db := newReconcileTestStore(t)
	ctx := context.Background()
	target := "https://facebook.com/p/recon-orphan"

	// Ledger says succeeded but NO execution_attempts row exists for
	// this outbound_id (pre-Step 3 historical row). Reconcile cannot
	// independently verify — leave it alone, count as orphaned.
	seedLedgerRow(t, db, 3, 300, LedgerOutcomeSucceeded, target)

	report, err := db.ReconcileEngagement(ctx, 3)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.LedgerOrphaned != 1 {
		t.Fatalf("expected 1 orphaned, got %+v", report)
	}
	if report.LedgerCorrected != 0 {
		t.Fatalf("orphans must NOT be corrected; got %d", report.LedgerCorrected)
	}
}

func TestReconcileEngagement_OrgScoped(t *testing.T) {
	db := newReconcileTestStore(t)
	ctx := context.Background()

	// One bad row in org 4, one bad row in org 5. Reconciling org 4
	// must NOT touch org 5.
	seedLedgerRow(t, db, 4, 400, LedgerOutcomeSucceeded, "https://facebook.com/p/o4")
	seedAttemptRow(t, db, 4, 400, models.ExecutionContextDrift, "https://facebook.com/p/o4")
	seedLedgerRow(t, db, 5, 500, LedgerOutcomeSucceeded, "https://facebook.com/p/o5")
	seedAttemptRow(t, db, 5, 500, models.ExecutionContextDrift, "https://facebook.com/p/o5")

	report, err := db.ReconcileEngagement(ctx, 4)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.LedgerCorrected != 1 {
		t.Fatalf("org 4 should correct 1 row, got %+v", report)
	}
	// Org 5 row unchanged.
	entries, _ := db.ListActionLedger(ctx, 5, "", "https://facebook.com/p/o5", time.Time{}, 10)
	if entries[0].Outcome != LedgerOutcomeSucceeded {
		t.Fatalf("org 5 must NOT be touched; got %q", entries[0].Outcome)
	}
}

func TestReconcileEngagement_Idempotent(t *testing.T) {
	db := newReconcileTestStore(t)
	ctx := context.Background()
	target := "https://facebook.com/p/recon-idem"

	seedLedgerRow(t, db, 6, 600, LedgerOutcomeSucceeded, target)
	seedAttemptRow(t, db, 6, 600, models.ExecutionRedirectedFeed, target)

	first, err := db.ReconcileEngagement(ctx, 6)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.LedgerCorrected != 1 {
		t.Fatalf("first pass should correct 1; got %+v", first)
	}
	// Second pass — the row is already 'failed', so nothing more to do.
	second, err := db.ReconcileEngagement(ctx, 6)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.LedgerCorrected != 0 || second.LedgerScanned != 0 {
		t.Fatalf("second pass must be no-op; got %+v", second)
	}
}
