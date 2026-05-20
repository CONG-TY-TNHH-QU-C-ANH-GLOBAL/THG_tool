// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newTestStore(t *testing.T) *Store {
	return newSharedStore(t, "outbound.db")
}

// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): every
// queued outbound goes directly to OutboundApproved (planned) — no
// human-approval draft gate. Pre-this-change the queue defaulted to
// draft unless the org had outbound_mode='auto'. The legacy
// outbound_mode policy still exists in context for back-compat but
// the queue no longer consults it; the requestedAuto argument is a
// no-op.
func TestQueueOutboundForOrgGoesStraightToApproved(t *testing.T) {
	db := newTestStore(t)
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/1",
		Content:   "hello",
		AIModel:   "agent",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if !res.Decision.Allowed || res.ID == 0 {
		t.Fatalf("expected allowed insert, got %+v", res)
	}
	// Autonomous-first: no approval gate. Even without any
	// outbound_mode policy set, the row lands at approved.
	if res.ExecutionState != models.ExecPlanned {
		t.Fatalf("expected approved (planned) under autonomous-first; got %q", res.ExecutionState)
	}
}

func TestQueueOutboundForOrgIgnoresLegacyOptInPolicy(t *testing.T) {
	db := newTestStore(t)
	// Even if some legacy code sets outbound_mode='draft', the queue
	// MUST still land at approved — the policy is dead in autonomous-
	// first mode.
	if err := db.SetContext("org:1:outbound_mode", "draft"); err != nil {
		t.Fatal(err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "inbox",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/u/1",
		Content:   "hi",
		AIModel:   "agent",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if res.ExecutionState != models.ExecPlanned {
		t.Fatalf("legacy outbound_mode=draft must NOT downgrade in autonomous-first; got %q", res.ExecutionState)
	}
}

func TestClaimPlannedOutboundForOrgMovesToSendingOnce(t *testing.T) {
	db := newTestStore(t)
	if err := db.SetContext("org:1:outbound_mode", "auto"); err != nil {
		t.Fatal(err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/claim",
		Content:   "hi",
		AIModel:   "agent",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	claim, err := db.ClaimPlannedOutboundForOrg(1, res.ID, "worker-a", 0)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claim == nil || claim.ExecutionID == "" {
		t.Fatalf("claim must issue execution_id; got %+v", claim)
	}
	if claim.LeaseExpiry.IsZero() {
		t.Fatalf("claim must set lease_expiry; got %+v", claim)
	}
	msg, err := db.GetOutboundForOrg(1, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ExecutionState != models.ExecExecuting {
		t.Fatalf("expected executing after claim, got %q", msg.ExecutionState)
	}
	if msg.ExecutionID != claim.ExecutionID {
		t.Fatalf("stored execution_id %q != issued %q", msg.ExecutionID, claim.ExecutionID)
	}
	if _, err := db.ClaimPlannedOutboundForOrg(1, res.ID, "worker-b", 0); err == nil {
		t.Fatal("expected second claim to fail")
	}
	dup, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/claim",
		Content:   "duplicate",
		AIModel:   "agent",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Decision.Allowed {
		t.Fatal("sending rows must still block duplicate outbound targets")
	}
}

// Legacy auto_comment_mode keys are dead under autonomous-first.
// Every queue lands at approved regardless of whatever stale global
// state exists, so this test now verifies the new invariant: no
// global key (auto_comment_mode, org:N:auto_comment_mode, anything)
// has any effect on the queued status.
func TestQueueOutboundForOrgIgnoresGlobalAutoCommentMode(t *testing.T) {
	db := newTestStore(t)
	_ = db.SetContext("auto_comment_mode", "true")
	_ = db.SetContext("org:1:auto_comment_mode", "true")

	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     1,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 7,
		TargetURL: "https://facebook.com/p/2",
		Content:   "hi",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	// Autonomous-first: status is always approved (planned).
	if res.ExecutionState != models.ExecPlanned {
		t.Fatalf("queue must land at approved regardless of legacy keys; got %q", res.ExecutionState)
	}
}

func TestQueueOutboundForOrgBlocksDuplicateActiveTarget(t *testing.T) {
	db := newTestStore(t)
	first := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: models.PlatformFacebook,
		AccountID: 7, TargetURL: "https://facebook.com/p/dup", Content: "x",
	}
	if _, err := db.QueueOutboundForOrg(first, time.Hour); err != nil {
		t.Fatal(err)
	}

	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: models.PlatformFacebook,
		AccountID: 7, TargetURL: "https://facebook.com/p/dup", Content: "y",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Allowed {
		t.Fatal("expected duplicate to be blocked")
	}
	// Coordination Plane PR-1: per-account scoping for comment means
	// same-account duplicates surface the per-account reason. (Cross-account
	// for comment is now ALLOWED — amplification case. Cross-account dedup
	// kept for inbox; see TestQueueOutbound_InboxIsCrossAccountStrict.)
	if res.Decision.Reason != "duplicate_outbound_per_account" {
		t.Fatalf("expected duplicate_outbound_per_account, got %q", res.Decision.Reason)
	}
}

func TestQueueOutboundForOrgConcurrentRaceLastResortUnique(t *testing.T) {
	// The application-level guard plus the partial UNIQUE index together
	// must ensure two concurrent QueueOutboundForOrg calls with the same
	// (org_id, type, target_url) result in exactly one row in the DB.
	db := newTestStore(t)
	target := "https://facebook.com/p/race"
	const goroutines = 8

	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]OutboundQueueResult, goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = db.QueueOutboundForOrg(&models.OutboundMessage{
				OrgID: 2, Type: "comment", Platform: models.PlatformFacebook,
				AccountID: 9, TargetURL: target, Content: "race",
			}, time.Hour)
		}(i)
	}
	wg.Wait()

	allowed := 0
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d errored: %v", i, errs[i])
		}
		if results[i].Decision.Allowed {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("expected exactly one allowed insert, got %d", allowed)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Execution lease + idempotency token invariants (PRIORITY A + B)
// ─────────────────────────────────────────────────────────────────────

// helper: queue an approved-for-auto row and return its ID.
func queueApprovedRow(t *testing.T, db *Store, orgID int64, target string) int64 {
	t.Helper()
	// Org must be in auto mode for the queue to land directly at approved.
	if err := db.SetContext(fmt.Sprintf("org:%d:outbound_mode", orgID), "auto"); err != nil {
		t.Fatalf("set auto mode: %v", err)
	}
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: orgID, Type: "comment", Platform: models.PlatformFacebook,
		AccountID: 7, TargetURL: target, Content: "hi", AIModel: "agent",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if res.ExecutionState != models.ExecPlanned {
		t.Fatalf("expected approved status, got %q", res.ExecutionState)
	}
	return res.ID
}

func TestClaimIssuesUniqueExecutionIDAndLease(t *testing.T) {
	db := newSharedStore(t, "outbound_claim_idem.db")
	id1 := queueApprovedRow(t, db, 11, "https://facebook.com/p/idem-1")
	id2 := queueApprovedRow(t, db, 11, "https://facebook.com/p/idem-2")

	c1, err := db.ClaimPlannedOutboundForOrg(11, id1, "worker", 0)
	if err != nil {
		t.Fatalf("claim id1: %v", err)
	}
	c2, err := db.ClaimPlannedOutboundForOrg(11, id2, "worker", 0)
	if err != nil {
		t.Fatalf("claim id2: %v", err)
	}
	if c1.ExecutionID == "" || c2.ExecutionID == "" {
		t.Fatal("execution_id must be non-empty")
	}
	if c1.ExecutionID == c2.ExecutionID {
		t.Fatalf("execution_ids must be unique across claims; both got %q", c1.ExecutionID)
	}
	if !c1.LeaseExpiry.After(time.Now()) {
		t.Fatalf("lease_expiry must be in the future; got %v", c1.LeaseExpiry)
	}
}

func TestFinalizeOutboundAttempt_FirstWin(t *testing.T) {
	db := newSharedStore(t, "outbound_finalize_firstwin.db")
	id := queueApprovedRow(t, db, 12, "https://facebook.com/p/win")

	claim, err := db.ClaimPlannedOutboundForOrg(12, id, "worker", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	finalized, state, outcome, execID, err := db.FinalizeOutboundAttempt(context.Background(), 12, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !finalized {
		t.Fatalf("first finalize must succeed; got finalized=false state=%q outcome=%q execID=%q", state, outcome, execID)
	}
	if state != models.ExecFinished || outcome != models.VerifVerifiedSuccess || execID != claim.ExecutionID {
		t.Fatalf("unexpected finalize return: state=%q outcome=%q execID=%q", state, outcome, execID)
	}
}

// Idempotent replay: same execution_id, same outcome, second call after
// the first finalize. Must NOT re-finalize (returns finalized=false) but
// MUST report the current terminal state so handlers can return 200
// idempotent-OK without replaying side effects.
func TestFinalizeOutboundAttempt_IdempotentReplay(t *testing.T) {
	db := newSharedStore(t, "outbound_finalize_replay.db")
	id := queueApprovedRow(t, db, 13, "https://facebook.com/p/replay")

	claim, err := db.ClaimPlannedOutboundForOrg(13, id, "worker", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	// First win.
	finalized, _, _, _, err := db.FinalizeOutboundAttempt(context.Background(), 13, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil || !finalized {
		t.Fatalf("first finalize must succeed (finalized=%v err=%v)", finalized, err)
	}
	// Replay with SAME execution_id.
	finalized, state, outcome, execID, err := db.FinalizeOutboundAttempt(context.Background(), 13, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("replay finalize: %v", err)
	}
	if finalized {
		t.Fatal("replay must NOT re-finalize")
	}
	if state != models.ExecFinished || outcome != models.VerifVerifiedSuccess {
		t.Fatalf("replay must report current (state=finished outcome=verified_success); got state=%q outcome=%q", state, outcome)
	}
	if execID != claim.ExecutionID {
		t.Fatalf("replay must report current execution_id=%q; got %q", claim.ExecutionID, execID)
	}
}

// Stale execution_id: row was reset (lease expired) and re-claimed; the
// original executor's callback arrives with a stale token. CAS must
// reject — finalized=false AND currentExecID is the NEW token.
func TestFinalizeOutboundAttempt_StaleExecutionID(t *testing.T) {
	db := newSharedStore(t, "outbound_finalize_stale.db")
	id := queueApprovedRow(t, db, 14, "https://facebook.com/p/stale")

	claim1, err := db.ClaimPlannedOutboundForOrg(14, id, "worker-a", 0)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Force the lease to be in the past so ResetStaleSending evicts it.
	if _, err := db.db.Exec(`UPDATE outbound_messages SET lease_expiry = datetime('now', '-1 minute') WHERE id = ?`, id); err != nil {
		t.Fatalf("force lease expiry: %v", err)
	}
	if err := db.ResetStaleExecutingForOrg(14, time.Minute); err != nil {
		t.Fatalf("reset stale: %v", err)
	}
	// Re-claim — issues a NEW execution_id.
	claim2, err := db.ClaimPlannedOutboundForOrg(14, id, "worker-b", 0)
	if err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if claim1.ExecutionID == claim2.ExecutionID {
		t.Fatal("re-claim must issue a new execution_id")
	}
	// Now the FIRST executor (worker-a) finally reports — but its
	// execution_id is the OLD one. Must NOT finalize.
	finalized, _, _, execIDNow, err := db.FinalizeOutboundAttempt(context.Background(), 14, id, claim1.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("stale finalize: %v", err)
	}
	if finalized {
		t.Fatal("stale execution_id must NOT finalize")
	}
	if execIDNow != claim2.ExecutionID {
		t.Fatalf("returned exec_id should be the current owner's; got %q want %q", execIDNow, claim2.ExecutionID)
	}
}

// Legacy compat: a row claimed before execution_id existed has
// execution_id=''. The finalize CAS must still work with empty token
// (status-only check). After all extensions ship the new field this
// branch becomes vestigial; until then it's the rollout safety net.
func TestFinalizeOutboundAttempt_LegacyEmptyToken(t *testing.T) {
	db := newSharedStore(t, "outbound_finalize_legacy.db")
	id := queueApprovedRow(t, db, 15, "https://facebook.com/p/legacy")

	// Simulate a legacy-shaped sending row (claimed before execution_id
	// existed): claim through the new path then strip the token.
	if _, err := db.ClaimPlannedOutboundForOrg(15, id, "legacy", 0); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE outbound_messages SET execution_id = '' WHERE id = ?`, id); err != nil {
		t.Fatalf("strip token: %v", err)
	}

	// Extension reports with NO execution_id (legacy build). CAS treats
	// empty + empty as a state-only check.
	finalized, state, outcome, _, err := db.FinalizeOutboundAttempt(context.Background(), 15, id, "", models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("legacy finalize: %v", err)
	}
	if !finalized {
		t.Fatal("legacy empty-token finalize must succeed")
	}
	if state != models.ExecFinished || outcome != models.VerifVerifiedSuccess {
		t.Fatalf("legacy finalize must set finished/verified_success; got state=%q outcome=%q", state, outcome)
	}
}

// Lease-aware steal-back: a row whose lease has expired is reset to
// approved on the next ResetStaleSending call; an unexpired lease is
// PROTECTED even if claimed_at is older than the legacy staleAfter.
func TestResetStaleSending_LeaseAware(t *testing.T) {
	db := newSharedStore(t, "outbound_reset_lease.db")
	id := queueApprovedRow(t, db, 16, "https://facebook.com/p/lease")

	if _, err := db.ClaimPlannedOutboundForOrg(16, id, "worker", time.Hour); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Push claimed_at WAY into the past — under legacy semantics this
	// would be reset. Under lease semantics, the unexpired lease wins.
	if _, err := db.db.Exec(`UPDATE outbound_messages SET claimed_at = datetime('now', '-1 hour') WHERE id = ?`, id); err != nil {
		t.Fatalf("force old claimed_at: %v", err)
	}
	if err := db.ResetStaleExecutingForOrg(16, time.Minute); err != nil {
		t.Fatalf("reset: %v", err)
	}
	row, err := db.GetOutboundForOrg(16, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.ExecutionState != models.ExecExecuting {
		t.Fatalf("unexpired lease must protect from reset; got state=%q", row.ExecutionState)
	}

	// Now force lease expiry; reset should fire.
	if _, err := db.db.Exec(`UPDATE outbound_messages SET lease_expiry = datetime('now', '-1 minute') WHERE id = ?`, id); err != nil {
		t.Fatalf("force lease expiry: %v", err)
	}
	if err := db.ResetStaleExecutingForOrg(16, time.Minute); err != nil {
		t.Fatalf("reset 2: %v", err)
	}
	row, err = db.GetOutboundForOrg(16, id)
	if err != nil {
		t.Fatalf("get 2: %v", err)
	}
	if row.ExecutionState != models.ExecPlanned {
		t.Fatalf("expired lease must reset to planned; got state=%q", row.ExecutionState)
	}
	if row.ExecutionID != "" {
		t.Fatalf("reset must clear execution_id; got %q", row.ExecutionID)
	}
}
