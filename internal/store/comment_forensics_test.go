// Domain: coordination forensics (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// Test #6: the forensics query returns the expected fields and classification for a
// submitted-but-unverified comment attempt (the 3-leads case).
func TestCommentForensics_ReturnsFieldsAndClassification(t *testing.T) {
	db := newSharedStore(t, "forensics.db")
	ctx := context.Background()
	postURL := "https://facebook.com/groups/1312868109620530/posts/2040011516906182/"

	// Real queue path → outbound + ledger row.
	res, err := db.Outbound().Queue(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "Bên mình nhận sourcing leather notebook",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, res.ID, models.LedgerOutcomeSubmittedUnverified, "no node match"); err != nil {
		t.Fatalf("mark ledger: %v", err)
	}

	// An execution_attempt with optimistic_success + persisted evidence.
	attemptID, err := db.Coordination().BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID: 1, OutboundID: res.ID, AccountID: 10, TargetURL: postURL,
		ActionType: "comment", Attempt: 1, Status: models.AttemptSubmitted,
	})
	if err != nil {
		t.Fatalf("begin attempt: %v", err)
	}
	if err := db.Coordination().FinishExecutionAttempt(ctx, attemptID, models.ExecutionOptimisticSuccess, "",
		coordination.VerificationEvidence{
			PageURLAfter: postURL,
			Notes:        "count+composer proof only; no node match",
		}); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rows, err := db.Coordination().CommentForensicsByTargetURLs(ctx, 1, []string{postURL})
	if err != nil {
		t.Fatalf("CommentForensicsByTargetURLs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.OutboundID != res.ID {
		t.Errorf("outbound_id = %d, want %d", r.OutboundID, res.ID)
	}
	if r.TargetURL != postURL {
		t.Errorf("target_url = %q", r.TargetURL)
	}
	if r.LedgerOutcome != models.LedgerOutcomeSubmittedUnverified {
		t.Errorf("ledger_outcome = %q, want submitted_unverified", r.LedgerOutcome)
	}
	if r.AttemptOutcome != string(models.ExecutionOptimisticSuccess) {
		t.Errorf("attempt_outcome = %q, want optimistic_success", r.AttemptOutcome)
	}
	if r.Classification != models.ForensicsSubmittedUnverified {
		t.Errorf("classification = %q, want %q", r.Classification, models.ForensicsSubmittedUnverified)
	}
	if !r.SubmitReached || !r.ComposerClearedAfterSubmit {
		t.Errorf("submit_reached/composer_cleared should be true for optimistic_success: %+v", r)
	}
	if r.VerifierFoundComment {
		t.Errorf("verifier_found_comment should be false (optimistic, not dom_verified)")
	}
	if r.PageURLAfter != postURL {
		t.Errorf("page_url_after = %q, want evidence value", r.PageURLAfter)
	}
}

// Classifier covers the 5 triage buckets the report needs.
func TestClassifyCommentForensics_Buckets(t *testing.T) {
	cases := map[string]string{
		"dom_verified":       models.ForensicsVerified,
		"optimistic_success": models.ForensicsSubmittedUnverified,
		"shadow_rejected":    models.ForensicsLikelyVerifyFalseNeg, // submit reached, verify missed
		"blocked":            models.ForensicsRealFailed,
		"rate_limited":       models.ForensicsRealFailed,
		"redirected_feed":    models.ForensicsRedirectedDrift,
		"context_drift":      models.ForensicsRedirectedDrift,
		"target_not_reached": models.ForensicsFailedBeforeSubmit,
		"composer_failed":    models.ForensicsFailedBeforeSubmit,
	}
	for outcome, want := range cases {
		if got := models.ClassifyCommentForensics(outcome); got != want {
			t.Errorf("ClassifyCommentForensics(%q) = %q, want %q", outcome, got, want)
		}
	}
}

// PR-A.1: the forensics report reflects the reverify pipeline state + the appended
// correction — so a stuck row can be traced and a corrected one reads as effectively
// succeeded even though the original outbound row stays submitted_unverified.
func TestReverify_ForensicsReflectsCorrection(t *testing.T) {
	db := newSharedStore(t, "reverify_forensics.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/RVF"
	seedSubmittedUnverified(t, db, 1, 10, postURL, "hello there")

	before, err := co.CommentForensicsByTargetURLs(ctx, 1, []string{postURL})
	if err != nil {
		t.Fatalf("forensics before: %v", err)
	}
	if before[0].ReverifyScheduled {
		t.Errorf("no reverify scheduled yet, want reverify_scheduled=false")
	}
	if before[0].LatestEffectiveOutcome != models.LedgerOutcomeSubmittedUnverified {
		t.Errorf("latest_effective_outcome = %q, want submitted_unverified", before[0].LatestEffectiveOutcome)
	}

	jobs, _ := co.FindReverifyEligible(ctx, time.Now().Add(time.Hour), 50)
	for _, j := range jobs {
		_ = co.ScheduleReverify(ctx, j, time.Now())
	}
	claimed, _ := co.ClaimDueReverifies(ctx, 1, 10, 7, time.Now(), 10)
	if _, err := co.ApplyReverifyResult(ctx, 1, claimed[0].ID, true, "https://fb.com/x?comment_id=1", "found"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after, err := co.CommentForensicsByTargetURLs(ctx, 1, []string{postURL})
	if err != nil {
		t.Fatalf("forensics after: %v", err)
	}
	r := after[0]
	if !r.ReverifyScheduled || r.ReverifyOutcome != "verified" {
		t.Errorf("reverify state wrong: scheduled=%v outcome=%q", r.ReverifyScheduled, r.ReverifyOutcome)
	}
	if r.CorrectionEventID <= 0 {
		t.Errorf("correction_event_id should be set after a found reverify")
	}
	if r.LatestEffectiveOutcome != "succeeded" {
		t.Errorf("latest_effective_outcome = %q, want succeeded", r.LatestEffectiveOutcome)
	}
}
