// Domain: coordination (see internal/store/DOMAINS.md)
package coordination_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

func newAttemptsTestStore(t *testing.T) (*store.Store, *coordination.Store) {
	return newCoordinationStore(t, "attempts.db")
}

// Step 3 invariant: an attempt's outcome is the single write-point for
// "what the platform did." Begin opens the row in `verifying`; Finish
// terminates with the verifier's classification + evidence. dom_verified
// must propagate as a flag for the badge / orchestrator read-side.
func TestExecutionAttempt_BeginFinish_DOMVerified(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	id, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      1,
		OutboundID: 100,
		AccountID:  10,
		TargetURL:  "https://facebook.com/groups/1/posts/100",
		ActionType: "comment",
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		t.Fatalf("BeginExecutionAttempt: %v", err)
	}
	if id <= 0 {
		t.Fatalf("BeginExecutionAttempt id=%d", id)
	}

	if err := db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", coordination.VerificationEvidence{
		CommentPermalink: "https://facebook.com/groups/1/posts/100?comment_id=999",
		DOMSnippet:       "<div>Alice: nice post</div>",
		PageURLAfter:     "https://facebook.com/groups/1/posts/100",
		ObservedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("FinishExecutionAttempt: %v", err)
	}

	got, err := db.GetExecutionAttempt(ctx, id)
	if err != nil {
		t.Fatalf("GetExecutionAttempt: %v", err)
	}
	if got.Outcome != models.ExecutionDOMVerified {
		t.Errorf("outcome = %q; want %q", got.Outcome, models.ExecutionDOMVerified)
	}
	if !got.DOMVerified {
		t.Errorf("dom_verified flag should be true for ExecutionDOMVerified")
	}
	if got.Status != models.AttemptDOMVerified {
		t.Errorf("terminal status = %q; want %q", got.Status, models.AttemptDOMVerified)
	}
	var ev coordination.VerificationEvidence
	if err := json.Unmarshal([]byte(got.EvidenceJSON), &ev); err != nil {
		t.Fatalf("evidence_json parse: %v", err)
	}
	if ev.CommentPermalink == "" {
		t.Errorf("evidence comment_permalink not persisted")
	}
}

// Shadow-reject is the most dangerous failure mode: extension reports
// success but verifier finds no DOM proof. The row must terminate with
// status=failed AND dom_verified=false — anything else would corrupt the
// badge / risk_score downstream.
func TestExecutionAttempt_ShadowRejected_NotMarkedVerified(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	id, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 200, AccountID: 10,
		ActionType: "comment", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := db.FinishExecutionAttempt(ctx, id, models.ExecutionShadowRejected, "no DOM proof", coordination.VerificationEvidence{}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got, _ := db.GetExecutionAttempt(ctx, id)
	if got.DOMVerified {
		t.Fatalf("shadow_rejected must NOT have dom_verified=true (this is the hallucination bug)")
	}
	if got.Status != models.AttemptFailed {
		t.Errorf("terminal status = %q; want %q", got.Status, models.AttemptFailed)
	}
	if got.FailureReason == "" {
		t.Errorf("failure_reason should be persisted")
	}
}

// Retries APPEND attempts — they never overwrite. The orchestrator reads
// the attempt chain to decide whether to keep trying (high retry count is
// itself a risk signal).
func TestExecutionAttempt_RetriesAppend(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	first, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 300, AccountID: 10, ActionType: "comment", Attempt: 1,
	})
	_ = db.FinishExecutionAttempt(ctx, first, models.ExecutionSoftFail, "network blip", coordination.VerificationEvidence{})

	second, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 300, AccountID: 10, ActionType: "comment", Attempt: 2,
	})
	_ = db.FinishExecutionAttempt(ctx, second, models.ExecutionDOMVerified, "", coordination.VerificationEvidence{
		CommentPermalink: "x",
	})

	all, err := db.ListAttemptsForOutbound(ctx, 300)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(attempts) = %d; want 2", len(all))
	}
	// Newest-first ordering (attempt DESC) lets the dashboard show latest at top.
	if all[0].Attempt != 2 {
		t.Errorf("first row attempt = %d; want 2 (newest-first)", all[0].Attempt)
	}
}

// MarkActionLedgerOutcomeByOutbound is the wire between verifier and the
// existing action_ledger surface. Must update by outbound_id and return
// the linked ledger_id so the attempt row can store the linkage.
func TestMarkActionLedgerOutcomeByOutbound(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	// Seed a queued ledger entry directly (skip the QueueOutboundForOrg
	// path — this test focuses on the by-outbound lookup specifically).
	ledgerID, err := db.RecordActionLedger(ctx, coordination.ActionLedgerEntry{
		OrgID: 1, ActionType: "comment", TargetURL: "https://fb.com/x",
		AccountID: 10, OutboundID: 500, Outcome: coordination.LedgerOutcomeQueued,
	})
	if err != nil {
		t.Fatalf("RecordActionLedger: %v", err)
	}

	gotID, err := db.MarkActionLedgerOutcomeByOutbound(ctx, 1, 500, "succeeded", "dom_verified")
	if err != nil {
		t.Fatalf("MarkActionLedgerOutcomeByOutbound: %v", err)
	}
	if gotID != ledgerID {
		t.Errorf("returned ledger_id = %d; want %d", gotID, ledgerID)
	}

	// Verify the row was actually updated.
	entries, _ := db.ListActionLedger(ctx, 1, "comment", "https://fb.com/x", time.Time{}, 10)
	if len(entries) != 1 {
		t.Fatalf("entries len = %d; want 1", len(entries))
	}
	if entries[0].Outcome != "succeeded" {
		t.Errorf("outcome = %q; want succeeded", entries[0].Outcome)
	}
	if entries[0].Reason != "dom_verified" {
		t.Errorf("reason = %q; want dom_verified", entries[0].Reason)
	}
}

// Lookup with no matching outbound row returns (0, nil) — defensive
// behaviour for manually-sent outbounds that bypassed the queue. Callers
// must NOT treat 0-id as a write failure.
func TestMarkActionLedgerOutcomeByOutbound_MissingReturnsZero(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	id, err := db.MarkActionLedgerOutcomeByOutbound(ctx, 1, 99999, "failed", "shadow_rejected")
	if err != nil {
		t.Fatalf("expected no error on missing row; got %v", err)
	}
	if id != 0 {
		t.Fatalf("expected ledger_id=0 on no match; got %d", id)
	}
}

// ── Step 4a observability queries ────────────────────────────────────────────

func TestExecutionOutcomeDistribution_GroupsAndBoundsTime(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	for i, outcome := range []models.ExecutionOutcome{
		models.ExecutionDOMVerified, models.ExecutionDOMVerified,
		models.ExecutionShadowRejected, models.ExecutionDOMVerified,
	} {
		actionType := "comment"
		if i == 3 {
			actionType = "inbox"
		}
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: 1, OutboundID: int64(1000 + i), AccountID: 10,
			ActionType: actionType, Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, outcome, "", coordination.VerificationEvidence{})
	}
	_, _ = db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 9999, AccountID: 10,
		ActionType: "comment", Attempt: 1,
	})

	buckets, err := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("distribution: %v", err)
	}
	total := 0
	for _, b := range buckets {
		total += b.Count
		if b.Outcome == "" {
			t.Errorf("unclassified rows leaked into distribution")
		}
	}
	if total != 4 {
		t.Fatalf("expected 4 classified attempts in distribution; got %d", total)
	}

	future, _ := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(time.Hour))
	if len(future) != 0 {
		t.Errorf("future-bounded query should be empty; got %d buckets", len(future))
	}
}

func TestExecutionOutcomeDistribution_OrgScoped(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	for _, orgID := range []int64{1, 1, 2} {
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: orgID, AccountID: 10, ActionType: "comment", Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", coordination.VerificationEvidence{})
	}
	org1, _ := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(-1*time.Hour))
	org2, _ := db.ExecutionOutcomeDistribution(ctx, 2, time.Now().Add(-1*time.Hour))
	sum := func(bs []coordination.OutcomeDistributionBucket) int {
		s := 0
		for _, b := range bs {
			s += b.Count
		}
		return s
	}
	if sum(org1) != 2 || sum(org2) != 1 {
		t.Errorf("org isolation broken: org1=%d org2=%d (want 2/1)", sum(org1), sum(org2))
	}
}

func TestListRecentExecutionAttempts_NewestFirstAndBounded(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: 1, OutboundID: int64(500 + i), AccountID: 10,
			ActionType: "comment", Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", coordination.VerificationEvidence{
			CommentPermalink: "perma-" + string(rune('A'+i)),
		})
	}
	rows, err := db.ListRecentExecutionAttempts(ctx, 1, time.Now().Add(-1*time.Hour), 3)
	if err != nil {
		t.Fatalf("ListRecentExecutionAttempts: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("limit not honoured: got %d, want 3", len(rows))
	}
	for i := 1; i < len(rows); i += 1 {
		if rows[i-1].OutboundID < rows[i].OutboundID {
			t.Errorf("not newest-first: rows[%d].OutboundID=%d < rows[%d].OutboundID=%d",
				i-1, rows[i-1].OutboundID, i, rows[i].OutboundID)
		}
	}
}

func TestAccountHealthSnapshot_OrderedByRiskDesc(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	for _, accountID := range []int64{100, 200, 300} {
		_ = db.ApplyRiskSignal(ctx, 1, accountID, models.RiskSignalSuccess, 0)
	}
	for i := 0; i < 3; i++ {
		_ = db.ApplyRiskSignal(ctx, 1, 200, models.RiskSignalFailure, 0)
	}
	_ = db.ApplyRiskSignal(ctx, 1, 300, models.RiskSignalCaptcha, 0)

	rows, err := db.AccountHealthSnapshot(ctx, 1, 0)
	if err != nil {
		t.Fatalf("AccountHealthSnapshot: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 accounts; got %d", len(rows))
	}
	for i := 1; i < len(rows); i += 1 {
		if rows[i-1].RiskScore < rows[i].RiskScore {
			t.Errorf("rows not sorted by risk DESC: idx %d=%.3f < idx %d=%.3f",
				i-1, rows[i-1].RiskScore, i, rows[i].RiskScore)
		}
	}
	if rows[0].AccountID != 200 {
		t.Errorf("expected account 200 at top; got %d", rows[0].AccountID)
	}
}

func TestAccountHealthSnapshot_SingleAccountFilter(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	_ = db.ApplyRiskSignal(ctx, 1, 100, models.RiskSignalSuccess, 0)
	_ = db.ApplyRiskSignal(ctx, 1, 200, models.RiskSignalFailure, 0)
	rows, err := db.AccountHealthSnapshot(ctx, 1, 200)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(rows) != 1 || rows[0].AccountID != 200 {
		t.Errorf("filter to account 200 failed: %+v", rows)
	}
}

// ── PR-E: stuck-state observation queries ────────────────────────────────────

// insertOutboundDirect bypasses the canonical write path so tests can
// plant rows with arbitrary timestamps + states. Uses the raw *sql.DB
// from the parent store handle (coordination.Store has its own DB()
// accessor that returns the same underlying connection).
func insertOutboundDirect(t *testing.T, db *sql.DB, orgID, accountID int64, msgType, targetURL, execState string, createdAt time.Time) int64 {
	t.Helper()
	ts := createdAt.UTC().Format("2006-01-02 15:04:05")
	res, err := db.Exec(
		`INSERT INTO outbound_messages
		   (org_id, type, platform, account_id, target_url, content, status, execution_state, created_at)
		 VALUES (?, ?, 'facebook', ?, ?, 'test content', 'draft', ?, ?)`,
		orgID, msgType, accountID, targetURL, execState, ts,
	)
	if err != nil {
		t.Fatalf("insert outbound: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestGapDetection_SurfacesStuckOutboundWithNoAttempts(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	const orgID = int64(99)
	now := time.Now().UTC()
	raw := db.DB()

	stuckID := insertOutboundDirect(t, raw, orgID, 1, "comment", "https://fb.com/groups/1/posts/stuck", "planned", now.Add(-30*time.Minute))
	healthyID := insertOutboundDirect(t, raw, orgID, 1, "comment", "https://fb.com/groups/1/posts/healthy", "planned", now.Add(-30*time.Minute))
	freshID := insertOutboundDirect(t, raw, orgID, 1, "comment", "https://fb.com/groups/1/posts/fresh", "planned", now.Add(-2*time.Minute))
	finishedID := insertOutboundDirect(t, raw, orgID, 1, "comment", "https://fb.com/groups/1/posts/finished", "finished", now.Add(-30*time.Minute))

	_, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      orgID,
		OutboundID: healthyID,
		AccountID:  1,
		TargetURL:  "https://fb.com/groups/1/posts/healthy",
		ActionType: "comment",
		Attempt:    1,
	})
	if err != nil {
		t.Fatalf("attempt for healthy: %v", err)
	}

	rows, err := db.GapDetection(ctx, orgID, now.Add(-10*time.Minute), 50)
	if err != nil {
		t.Fatalf("GapDetection: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 stuck row, got %d: %+v", len(rows), rows)
	}
	if rows[0].OutboundID != stuckID {
		t.Errorf("stuck id = %d, want %d (healthy=%d fresh=%d finished=%d)",
			rows[0].OutboundID, stuckID, healthyID, freshID, finishedID)
	}
	if rows[0].AgeSeconds < 60 {
		t.Errorf("age_seconds = %d, expected at least 60s for a 30-min-old row", rows[0].AgeSeconds)
	}
}

func TestGapDetection_TenantScoped(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	raw := db.DB()

	insertOutboundDirect(t, raw, 1, 1, "comment", "https://fb.com/a", "planned", now.Add(-30*time.Minute))
	insertOutboundDirect(t, raw, 2, 1, "comment", "https://fb.com/b", "planned", now.Add(-30*time.Minute))

	rows, err := db.GapDetection(ctx, 1, now.Add(-10*time.Minute), 50)
	if err != nil {
		t.Fatalf("GapDetection: %v", err)
	}
	if len(rows) != 1 || rows[0].OrgID != 1 {
		t.Errorf("tenant scoping failed: %+v", rows)
	}
}

func TestAccountOutcomeTimeseries_BucketsByHour(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	const orgID = int64(5)
	const accountID = int64(77)

	for i, outcome := range []models.ExecutionOutcome{
		models.ExecutionDOMVerified,
		models.ExecutionDOMVerified,
		models.ExecutionShadowRejected,
	} {
		id, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID:      orgID,
			OutboundID: int64(500 + i),
			AccountID:  accountID,
			TargetURL:  "https://fb.com/x",
			ActionType: "comment",
			Attempt:    1,
		})
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := db.FinishExecutionAttempt(ctx, id, outcome, "", coordination.VerificationEvidence{}); err != nil {
			t.Fatalf("Finish %d: %v", i, err)
		}
	}

	buckets, err := db.AccountOutcomeTimeseries(ctx, orgID, accountID, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("AccountOutcomeTimeseries: %v", err)
	}
	counts := map[string]int{}
	for _, b := range buckets {
		counts[b.Outcome] += b.Count
	}
	if counts[string(models.ExecutionDOMVerified)] != 2 {
		t.Errorf("dom_verified count = %d, want 2; buckets=%+v", counts[string(models.ExecutionDOMVerified)], buckets)
	}
	if counts[string(models.ExecutionShadowRejected)] != 1 {
		t.Errorf("shadow_rejected count = %d, want 1", counts[string(models.ExecutionShadowRejected)])
	}
}

func TestAccountOutcomeTimeseries_RequiresAccountID(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	if _, err := db.AccountOutcomeTimeseries(context.Background(), 1, 0, time.Now()); err == nil {
		t.Fatal("expected error when account_id missing")
	}
}

func TestLedgerReconcileMismatches_FlagsHallucinatedSuccess(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()
	const orgID = int64(11)
	const accountID = int64(22)

	setups := []struct {
		outboundID int64
		outcome    models.ExecutionOutcome
	}{
		{outboundID: 1001, outcome: models.ExecutionDOMVerified},
		{outboundID: 1002, outcome: models.ExecutionShadowRejected},
		{outboundID: 1003, outcome: models.ExecutionBlocked},
	}
	for _, s := range setups {
		id, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID:      orgID,
			OutboundID: s.outboundID,
			AccountID:  accountID,
			TargetURL:  "https://fb.com/p",
			ActionType: "comment",
			Attempt:    1,
		})
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := db.FinishExecutionAttempt(ctx, id, s.outcome, "", coordination.VerificationEvidence{}); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if _, err := db.RecordActionLedger(ctx, coordination.ActionLedgerEntry{
			OrgID:      orgID,
			ActionType: "comment",
			TargetURL:  "https://fb.com/p",
			AccountID:  accountID,
			OutboundID: s.outboundID,
			Outcome:    coordination.LedgerOutcomeSucceeded,
		}); err != nil {
			t.Fatalf("RecordActionLedger: %v", err)
		}
	}

	mismatches, err := db.LedgerReconcileMismatches(ctx, orgID, time.Now().UTC().Add(-1*time.Hour), 50)
	if err != nil {
		t.Fatalf("LedgerReconcileMismatches: %v", err)
	}
	if len(mismatches) != 2 {
		t.Fatalf("want 2 mismatches, got %d: %+v", len(mismatches), mismatches)
	}
	got := map[int64]string{}
	for _, m := range mismatches {
		got[m.OutboundID] = m.AttemptOutcome
		if m.LedgerOutcome != coordination.LedgerOutcomeSucceeded {
			t.Errorf("ledger_outcome = %q, want succeeded", m.LedgerOutcome)
		}
	}
	if got[1002] != string(models.ExecutionShadowRejected) {
		t.Errorf("outbound 1002 attempt outcome = %q, want shadow_rejected", got[1002])
	}
	if got[1003] != string(models.ExecutionBlocked) {
		t.Errorf("outbound 1003 attempt outcome = %q, want blocked", got[1003])
	}
	if _, found := got[1001]; found {
		t.Error("outbound 1001 (dom_verified) should NOT be flagged as mismatch")
	}
}

func TestLedgerReconcileMismatches_TenantScoped(t *testing.T) {
	_, db := newAttemptsTestStore(t)
	ctx := context.Background()

	for _, orgID := range []int64{1, 2} {
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: orgID, OutboundID: 5000 + orgID, AccountID: 1, TargetURL: "https://fb.com/p", ActionType: "comment", Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, models.ExecutionShadowRejected, "", coordination.VerificationEvidence{})
		_, _ = db.RecordActionLedger(ctx, coordination.ActionLedgerEntry{
			OrgID: orgID, ActionType: "comment", TargetURL: "https://fb.com/p",
			AccountID: 1, OutboundID: 5000 + orgID, Outcome: coordination.LedgerOutcomeSucceeded,
		})
	}

	rows, err := db.LedgerReconcileMismatches(ctx, 1, time.Now().UTC().Add(-1*time.Hour), 50)
	if err != nil {
		t.Fatalf("LedgerReconcileMismatches: %v", err)
	}
	if len(rows) != 1 || rows[0].OrgID != 1 {
		t.Errorf("tenant scoping failed: %+v", rows)
	}
}
