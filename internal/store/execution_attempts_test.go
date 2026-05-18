package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newAttemptsTestStore(t *testing.T) *Store {
	return newSharedStore(t, "attempts.db")
}

// Step 3 invariant: an attempt's outcome is the single write-point for
// "what the platform did." Begin opens the row in `verifying`; Finish
// terminates with the verifier's classification + evidence. dom_verified
// must propagate as a flag for the badge / orchestrator read-side.
func TestExecutionAttempt_BeginFinish_DOMVerified(t *testing.T) {
	db := newAttemptsTestStore(t)
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

	if err := db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", VerificationEvidence{
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
	var ev VerificationEvidence
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
	db := newAttemptsTestStore(t)
	ctx := context.Background()

	id, err := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 200, AccountID: 10,
		ActionType: "comment", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := db.FinishExecutionAttempt(ctx, id, models.ExecutionShadowRejected, "no DOM proof", VerificationEvidence{}); err != nil {
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
	db := newAttemptsTestStore(t)
	ctx := context.Background()

	first, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 300, AccountID: 10, ActionType: "comment", Attempt: 1,
	})
	_ = db.FinishExecutionAttempt(ctx, first, models.ExecutionSoftFail, "network blip", VerificationEvidence{})

	second, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 300, AccountID: 10, ActionType: "comment", Attempt: 2,
	})
	_ = db.FinishExecutionAttempt(ctx, second, models.ExecutionDOMVerified, "", VerificationEvidence{
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
	db := newAttemptsTestStore(t)
	ctx := context.Background()

	// Seed a queued ledger entry directly (skip the QueueOutboundForOrg
	// path — this test focuses on the by-outbound lookup specifically).
	ledgerID, err := db.RecordActionLedger(ctx, ActionLedgerEntry{
		OrgID: 1, ActionType: "comment", TargetURL: "https://fb.com/x",
		AccountID: 10, OutboundID: 500, Outcome: LedgerOutcomeQueued,
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
	db := newAttemptsTestStore(t)
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

// ExecutionOutcomeDistribution must include only classified rows (outcome != '')
// and must respect the time window. Used by the dashboard "outcome distribution"
// panel — a stale or unbounded query here would mislead the operator about
// the current health of execution.
func TestExecutionOutcomeDistribution_GroupsAndBoundsTime(t *testing.T) {
	db := newAttemptsTestStore(t)
	ctx := context.Background()

	// Seed: 2 dom_verified (comment), 1 shadow_rejected (comment), 1 dom_verified (inbox),
	// plus 1 row with empty outcome that must be excluded.
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
		_ = db.FinishExecutionAttempt(ctx, id, outcome, "", VerificationEvidence{})
	}
	// Open attempt with no outcome — must NOT appear in distribution.
	_, _ = db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: 9999, AccountID: 10,
		ActionType: "comment", Attempt: 1,
	})

	buckets, err := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("distribution: %v", err)
	}
	// Expect 3 buckets: dom_verified×comment (2), shadow_rejected×comment (1), dom_verified×inbox (1)
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

	// Time window must exclude old rows. Use a future "since" to confirm
	// the window predicate works (no rows from the future).
	future, _ := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(time.Hour))
	if len(future) != 0 {
		t.Errorf("future-bounded query should be empty; got %d buckets", len(future))
	}
}

// Org isolation: distribution must NOT leak rows from another org. The
// dashboard is the most-visible org-bleed risk surface.
func TestExecutionOutcomeDistribution_OrgScoped(t *testing.T) {
	db := newAttemptsTestStore(t)
	ctx := context.Background()
	for _, orgID := range []int64{1, 1, 2} {
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: orgID, AccountID: 10, ActionType: "comment", Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", VerificationEvidence{})
	}
	org1, _ := db.ExecutionOutcomeDistribution(ctx, 1, time.Now().Add(-1*time.Hour))
	org2, _ := db.ExecutionOutcomeDistribution(ctx, 2, time.Now().Add(-1*time.Hour))
	sum := func(bs []OutcomeDistributionBucket) int {
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

// ListRecentExecutionAttempts returns the parsed evidence in newest-first
// order, capped at limit. The dashboard renders this directly so the
// ordering invariant is load-bearing.
func TestListRecentExecutionAttempts_NewestFirstAndBounded(t *testing.T) {
	db := newAttemptsTestStore(t)
	ctx := context.Background()
	// Insert 5 rows; verify order + limit.
	for i := 0; i < 5; i++ {
		id, _ := db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
			OrgID: 1, OutboundID: int64(500 + i), AccountID: 10,
			ActionType: "comment", Attempt: 1,
		})
		_ = db.FinishExecutionAttempt(ctx, id, models.ExecutionDOMVerified, "", VerificationEvidence{
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
	// Newest first: started_at descends. Each row has a unique outbound_id
	// inserted in ascending order, so a correctly-sorted result has
	// outbound_id strictly descending.
	for i := 1; i < len(rows); i += 1 {
		if rows[i-1].OutboundID < rows[i].OutboundID {
			t.Errorf("not newest-first: rows[%d].OutboundID=%d < rows[%d].OutboundID=%d",
				i-1, rows[i-1].OutboundID, i, rows[i].OutboundID)
		}
	}
}

// AccountHealthSnapshot orders by risk_score DESC so poisoned accounts
// surface first on the dashboard — the most-poisoned account is the
// signal an operator needs to see immediately.
func TestAccountHealthSnapshot_OrderedByRiskDesc(t *testing.T) {
	db := newAttemptsTestStore(t)
	ctx := context.Background()

	// Seed runtime state for 3 accounts with different risk levels by
	// applying success/failure signals. ApplyRiskSignal handles the
	// runtime_state upsert.
	for _, accountID := range []int64{100, 200, 300} {
		_ = db.ApplyRiskSignal(ctx, 1, accountID, models.RiskSignalSuccess, 0)
	}
	// account 200 collects 3 failures → high risk
	for i := 0; i < 3; i++ {
		_ = db.ApplyRiskSignal(ctx, 1, 200, models.RiskSignalFailure, 0)
	}
	// account 300 gets 1 captcha → elevated risk
	_ = db.ApplyRiskSignal(ctx, 1, 300, models.RiskSignalCaptcha, 0)

	rows, err := db.AccountHealthSnapshot(ctx, 1, 0)
	if err != nil {
		t.Fatalf("AccountHealthSnapshot: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 accounts; got %d", len(rows))
	}
	// Highest risk first.
	for i := 1; i < len(rows); i += 1 {
		if rows[i-1].RiskScore < rows[i].RiskScore {
			t.Errorf("rows not sorted by risk DESC: idx %d=%.3f < idx %d=%.3f",
				i-1, rows[i-1].RiskScore, i, rows[i].RiskScore)
		}
	}
	// account 200 (3 failures) should be at the top.
	if rows[0].AccountID != 200 {
		t.Errorf("expected account 200 at top; got %d", rows[0].AccountID)
	}
}

// AccountHealthSnapshot filters to a single account when account_id > 0.
func TestAccountHealthSnapshot_SingleAccountFilter(t *testing.T) {
	db := newAttemptsTestStore(t)
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
