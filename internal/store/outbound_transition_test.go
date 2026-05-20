// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Helpers ---------------------------------------------------------------

// queueOnePlanned enqueues a single comment outbound for the given org +
// account and returns its row ID. Centralised here so the assertions
// below can stay focused on the transition-ledger invariants.
func queueOnePlanned(t *testing.T, db *Store, orgID, accountID int64, target string) int64 {
	t.Helper()
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     orgID,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: accountID,
		TargetURL: target,
		Content:   "hi",
		AIModel:   "agent",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !res.Decision.Allowed || res.ID == 0 {
		t.Fatalf("queue refused: %+v", res)
	}
	return res.ID
}

// countTransitions reports how many ledger rows of a given transition_type
// exist for an outbound id. The test uses this as the spec for
// "additive ledger writes succeeded".
func countTransitions(t *testing.T, db *Store, outboundID int64, transitionType TransitionType) int {
	t.Helper()
	var n int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM execution_attempts
		 WHERE outbound_id = ? AND transition_type = ?`,
		outboundID, string(transitionType),
	).Scan(&n); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	return n
}

// Transition-row append assertions ---------------------------------------

// TestTransitionLedger_PlanRowEmittedOnQueue asserts that every successful
// QueueOutboundForOrg appends exactly one 'plan' transition with the
// resulting_state denormalized to 'planned'. PR-2 (V2 staged refactor)
// invariant: the audit trail starts at queue time, not at claim time.
func TestTransitionLedger_PlanRowEmittedOnQueue(t *testing.T) {
	db := newSharedStore(t, "tx_ledger_plan.db")
	id := queueOnePlanned(t, db, 1, 7, "https://facebook.com/p/plan-ledger")

	if got := countTransitions(t, db, id, TransitionPlan); got != 1 {
		t.Fatalf("expected 1 plan transition, got %d", got)
	}
	// Verify the denormalized state was written.
	var state, outcome string
	if err := db.db.QueryRow(
		`SELECT resulting_state, COALESCE(resulting_outcome,'') FROM execution_attempts
		 WHERE outbound_id = ? AND transition_type = 'plan'`, id,
	).Scan(&state, &outcome); err != nil {
		t.Fatalf("read plan row: %v", err)
	}
	if state != string(models.ExecPlanned) || outcome != "" {
		t.Fatalf("plan row state mismatch: got state=%q outcome=%q want (planned, '')", state, outcome)
	}
}

// TestTransitionLedger_ClaimAndFinalizeAppendRows asserts the full
// plan → claim → finalize transition chain is recorded. Each phase
// emits exactly one ledger row, and the execution_id is propagated
// from the claim row into the finalize row for audit trail integrity.
func TestTransitionLedger_ClaimAndFinalizeAppendRows(t *testing.T) {
	db := newSharedStore(t, "tx_ledger_chain.db")
	id := queueOnePlanned(t, db, 2, 11, "https://facebook.com/p/chain-ledger")

	// Plan row already present from queue.
	if got := countTransitions(t, db, id, TransitionPlan); got != 1 {
		t.Fatalf("plan: expected 1, got %d", got)
	}

	claim, err := db.ClaimPlannedOutboundForOrg(2, id, "worker", 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got := countTransitions(t, db, id, TransitionClaim); got != 1 {
		t.Fatalf("claim: expected 1, got %d", got)
	}

	finalized, _, _, _, err := db.FinalizeOutboundAttempt(
		context.Background(), 2, id, claim.ExecutionID,
		models.ExecFinished, models.VerifVerifiedSuccess,
	)
	if err != nil || !finalized {
		t.Fatalf("finalize: finalized=%v err=%v", finalized, err)
	}
	if got := countTransitions(t, db, id, TransitionFinalize); got != 1 {
		t.Fatalf("finalize: expected 1, got %d", got)
	}

	// Idempotent replay must NOT append a second finalize row.
	finalized2, _, _, _, err := db.FinalizeOutboundAttempt(
		context.Background(), 2, id, claim.ExecutionID,
		models.ExecFinished, models.VerifVerifiedSuccess,
	)
	if err != nil {
		t.Fatalf("replay finalize: %v", err)
	}
	if finalized2 {
		t.Fatal("replay must NOT re-finalize")
	}
	if got := countTransitions(t, db, id, TransitionFinalize); got != 1 {
		t.Fatalf("replay must not append a 2nd finalize row; got %d", got)
	}

	// Execution_id must match between claim and finalize.
	var claimExecID, finalExecID string
	_ = db.db.QueryRow(
		`SELECT execution_id FROM execution_attempts WHERE outbound_id = ? AND transition_type = 'claim'`, id,
	).Scan(&claimExecID)
	_ = db.db.QueryRow(
		`SELECT execution_id FROM execution_attempts WHERE outbound_id = ? AND transition_type = 'finalize'`, id,
	).Scan(&finalExecID)
	if claimExecID == "" || claimExecID != finalExecID {
		t.Fatalf("execution_id audit chain broken: claim=%q finalize=%q", claimExecID, finalExecID)
	}
}

// TestTransitionLedger_ResetAppendsRow asserts that a lease-eviction
// reset emits a 'reset' transition row carrying the evicted
// execution_id (audit-only). The outbound row's projection flips back
// to planned via the existing CAS UPDATE.
func TestTransitionLedger_ResetAppendsRow(t *testing.T) {
	db := newSharedStore(t, "tx_ledger_reset.db")
	id := queueOnePlanned(t, db, 3, 12, "https://facebook.com/p/reset-ledger")

	if _, err := db.ClaimPlannedOutboundForOrg(3, id, "worker", 0); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Force the lease into the past so ResetStaleSending evicts.
	if _, err := db.db.Exec(`UPDATE outbound_messages SET lease_expiry = datetime('now', '-1 minute') WHERE id = ?`, id); err != nil {
		t.Fatalf("force lease expiry: %v", err)
	}
	if err := db.ResetStaleExecutingForOrg(3, time.Minute); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := countTransitions(t, db, id, TransitionReset); got != 1 {
		t.Fatalf("reset: expected 1, got %d", got)
	}
	// Verify the reset row's resulting_state is 'planned' (the projection
	// effectively re-opened the row).
	var state string
	_ = db.db.QueryRow(
		`SELECT resulting_state FROM execution_attempts WHERE outbound_id = ? AND transition_type = 'reset'`, id,
	).Scan(&state)
	if state != string(models.ExecPlanned) {
		t.Fatalf("reset row state want planned, got %q", state)
	}
}

// Policy-driven dedup A/B parity ----------------------------------------

// TestActionPolicy_CommentBlocksSameAccountSameTarget verifies the
// comment policy preserves the old hardcoded behaviour: a second
// enqueue from the same account against the same target_url is
// blocked with duplicate_outbound_per_account.
func TestActionPolicy_CommentBlocksSameAccountSameTarget(t *testing.T) {
	db := newSharedStore(t, "policy_comment_per_account.db")
	target := "https://facebook.com/p/policy-comment"

	first := &models.OutboundMessage{
		OrgID:     10,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 100,
		TargetURL: target,
		Content:   "first",
	}
	if _, err := db.QueueOutboundForOrg(first, time.Hour); err != nil {
		t.Fatalf("first queue: %v", err)
	}

	second, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     10,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 100,
		TargetURL: target,
		Content:   "second",
	}, time.Hour)
	if err != nil {
		t.Fatalf("second queue: %v", err)
	}
	if second.Decision.Allowed {
		t.Fatal("second comment from same account must be blocked by policy")
	}
	if second.Decision.Reason != "duplicate_outbound_per_account" {
		t.Fatalf("expected duplicate_outbound_per_account, got %q", second.Decision.Reason)
	}
}

// TestActionPolicy_InboxCrossAccountBlocks verifies the inbox policy
// preserves the spam-cluster prevention rule: two different accounts
// cannot inbox the same lead workspace-wide.
func TestActionPolicy_InboxCrossAccountBlocks(t *testing.T) {
	db := newSharedStore(t, "policy_inbox_crossaccount.db")
	leadURL := "https://facebook.com/u/policy-inbox"

	alice := &models.OutboundMessage{
		OrgID:     11,
		Type:      "inbox",
		Platform:  models.PlatformFacebook,
		AccountID: 200,
		TargetURL: leadURL,
		Content:   "alice",
	}
	if _, err := db.QueueOutboundForOrg(alice, time.Hour); err != nil {
		t.Fatalf("alice queue: %v", err)
	}

	bob, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     11,
		Type:      "inbox",
		Platform:  models.PlatformFacebook,
		AccountID: 201,
		TargetURL: leadURL,
		Content:   "bob",
	}, time.Hour)
	if err != nil {
		t.Fatalf("bob queue: %v", err)
	}
	if bob.Decision.Allowed {
		t.Fatal("inbox from a different account must be blocked workspace-wide (spam-cluster prevention)")
	}
	if bob.Decision.Reason != "duplicate_outbound_target" {
		t.Fatalf("expected duplicate_outbound_target (workspace scope), got %q", bob.Decision.Reason)
	}
}

// TestActionPolicy_CommentCrossAccountAmplificationAllowed verifies
// the comment policy preserves the coordinated-amplification rule:
// two DIFFERENT accounts commenting on the same post is allowed.
func TestActionPolicy_CommentCrossAccountAmplificationAllowed(t *testing.T) {
	db := newSharedStore(t, "policy_comment_amplification.db")
	postURL := "https://facebook.com/groups/p/policy-amp"

	alice := &models.OutboundMessage{
		OrgID:     12,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 300,
		TargetURL: postURL,
		Content:   "alice's take",
	}
	if r, err := db.QueueOutboundForOrg(alice, time.Hour); err != nil || !r.Decision.Allowed {
		t.Fatalf("alice queue failed: err=%v decision=%+v", err, r.Decision)
	}

	bob, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     12,
		Type:      "comment",
		Platform:  models.PlatformFacebook,
		AccountID: 301,
		TargetURL: postURL,
		Content:   "bob's take",
	}, time.Hour)
	if err != nil {
		t.Fatalf("bob queue: %v", err)
	}
	if !bob.Decision.Allowed {
		t.Fatalf("bob's comment must be allowed (amplification), got %q", bob.Decision.Reason)
	}
}

// TestActionPolicy_UnknownActionTypeRefused verifies the policy gate
// fails closed: an action type without a seeded action_policies row
// is refused with action_policy_missing. Future action types must
// land a row before the queue accepts them — the system never falls
// back to permissive defaults.
func TestActionPolicy_UnknownActionTypeRefused(t *testing.T) {
	db := newSharedStore(t, "policy_unknown_action.db")
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID:     13,
		Type:      "shouting", // no seed for this type
		Platform:  models.PlatformFacebook,
		AccountID: 400,
		TargetURL: "https://facebook.com/x/unknown",
		Content:   "hi",
	}, time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatal("queue must refuse unknown action type")
	}
	if res.Decision.Reason != "action_policy_missing" {
		t.Fatalf("expected action_policy_missing, got %q", res.Decision.Reason)
	}
}

// TestActionPolicy_OrgOverrideShadowsGlobalDefault verifies the
// resolver picks the org-specific row when both org-specific and
// global default exist for an action_type. The override is what
// makes the system multi-tenant-configurable from a single seed.
func TestActionPolicy_OrgOverrideShadowsGlobalDefault(t *testing.T) {
	db := newSharedStore(t, "policy_org_override.db")
	const orgID = int64(14)

	// Admin upserts an override that loosens dedup to 'none' for comment.
	if err := db.UpsertActionPolicy(context.Background(), ActionPolicy{
		OrgID:             orgID,
		ActionType:        "comment",
		DedupScope:        DedupScopeNone,
		BlockOnPlanned:    false,
		BlockOnExecuting:  false,
		CooldownSeconds:   0,
		ConversationAware: false,
	}); err != nil {
		t.Fatalf("upsert override: %v", err)
	}

	resolved, err := db.GetActionPolicy(context.Background(), orgID, "comment")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.OrgID != orgID {
		t.Fatalf("resolver picked wrong row: got org_id=%d want %d", resolved.OrgID, orgID)
	}
	if resolved.DedupScope != DedupScopeNone {
		t.Fatalf("override dedup_scope not applied: got %q", resolved.DedupScope)
	}

	// Verify a different org still sees the global default.
	def, err := db.GetActionPolicy(context.Background(), 999, "comment")
	if err != nil {
		t.Fatalf("default resolve: %v", err)
	}
	if def.OrgID != 0 || def.DedupScope != DedupScopePerAccount {
		t.Fatalf("unrelated org must see global default; got org_id=%d scope=%q", def.OrgID, def.DedupScope)
	}
}
