// Domain: coordination human-verify (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// seedComment queues a comment and forces a terminal verification_outcome (like finalize).
func seedComment(t *testing.T, db *Store, orgID, accountID int64, postURL, content, vo string) int64 {
	t.Helper()
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: orgID, Type: "comment", Platform: "facebook",
		AccountID: accountID, TargetURL: postURL, Content: content,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.db.Exec(
		`UPDATE outbound_messages SET verification_outcome=?, execution_state='finished' WHERE id=?`,
		vo, res.ID); err != nil {
		t.Fatalf("set %s: %v", vo, err)
	}
	if vo == "submitted_unverified" {
		_, _ = db.Coordination().MarkActionLedgerOutcomeByOutbound(context.Background(), orgID, res.ID, vo, "optimistic_success")
	}
	return res.ID
}

// Manual human-verify appends a 'succeeded'/'human_verified' correction (append-only — the
// old submitted_unverified row is untouched), writes the audit trail, and is idempotent.
func TestHumanVerify_AppendsCorrectionAuditedIdempotent(t *testing.T) {
	db := newSharedStore(t, "human_verify.db")
	ctx := context.Background()
	co := db.Coordination()
	postURL := "https://facebook.com/groups/1/posts/HV1"
	outboundID := seedComment(t, db, 1, 10, postURL, "Bên mình nhận sourcing", "submitted_unverified")

	in := coordination.HumanVerifyInput{
		OrgID: 1, OutboundID: outboundID, TargetURL: postURL, AccountID: 10,
		VerifiedBy: 42, PreviousOutcome: "submitted_unverified",
	}
	res, err := co.AppendHumanVerifyCorrection(ctx, in)
	if err != nil {
		t.Fatalf("AppendHumanVerifyCorrection: %v", err)
	}
	if !res.Corrected || res.NewEffectiveOutcome != "succeeded" {
		t.Fatalf("want corrected→succeeded, got %+v", res)
	}

	// Append-only: original submitted_unverified row remains; a succeeded row is added.
	outcomes := ledgerOutcomes(t, db, 1, postURL)
	if len(outcomes) != 2 {
		t.Fatalf("ledger rows = %v, want [submitted_unverified, succeeded]", outcomes)
	}
	// Effective outcome is now verified (a succeeded touch exists).
	if !models.IsLedgerOutcomeVerifiedTouch("succeeded") {
		t.Error("correction must count as a verified touch")
	}

	// Audit row recorded.
	var auditCount int
	_ = db.db.QueryRow(`SELECT COUNT(*) FROM comment_verification_audit WHERE org_id=1 AND outbound_id=? AND verified_by_user_id=42 AND source=?`,
		outboundID, models.HumanVerifySource).Scan(&auditCount)
	if auditCount != 1 {
		t.Errorf("audit rows = %d, want 1", auditCount)
	}

	// Idempotent: a second confirm is a no-op (no duplicate correction).
	again, _ := co.AppendHumanVerifyCorrection(ctx, in)
	if !again.AlreadyVerified || again.Corrected {
		t.Errorf("second confirm must be already_verified no-op, got %+v", again)
	}
	if got := ledgerOutcomes(t, db, 1, postURL); len(got) != 2 {
		t.Errorf("idempotent: ledger must stay 2 rows, got %v", got)
	}
}

// Forensics/metrics see the human-verify correction as effectively succeeded.
func TestCommentOutcomeMetrics(t *testing.T) {
	db := newSharedStore(t, "comment_metrics.db")
	ctx := context.Background()
	co := db.Coordination()
	seedComment(t, db, 1, 10, "https://fb.com/p/M1", "a", "verified_success")
	su := seedComment(t, db, 1, 10, "https://fb.com/p/M2", "b", "submitted_unverified")
	seedComment(t, db, 1, 10, "https://fb.com/p/M3", "c", "target_not_reached")
	seedComment(t, db, 1, 10, "https://fb.com/p/M4", "d", "execution_failed")
	// Manually verify the submitted_unverified one.
	if _, err := co.AppendHumanVerifyCorrection(ctx, coordination.HumanVerifyInput{
		OrgID: 1, OutboundID: su, TargetURL: "https://fb.com/p/M2", AccountID: 10, VerifiedBy: 42, PreviousOutcome: "submitted_unverified",
	}); err != nil {
		t.Fatalf("human verify: %v", err)
	}

	m, err := co.CommentOutcomeMetrics(ctx, 1, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("CommentOutcomeMetrics: %v", err)
	}
	if m.Total != 4 {
		t.Errorf("total = %d, want 4", m.Total)
	}
	if m.VerifiedSuccess != 1 || m.SubmittedUnverified != 1 || m.TargetNotReached != 1 || m.ExecutionFailed != 1 {
		t.Errorf("buckets wrong: %+v", m)
	}
	if m.HumanVerified != 1 {
		t.Errorf("human_verified = %d, want 1", m.HumanVerified)
	}
	if m.EffectiveVerified() != 2 { // verified_success + human_verified
		t.Errorf("effective verified = %d, want 2", m.EffectiveVerified())
	}
	if m.SubmittedUnverifiedOpen() != 0 { // the 1 submitted was human-verified
		t.Errorf("open = %d, want 0", m.SubmittedUnverifiedOpen())
	}
}
