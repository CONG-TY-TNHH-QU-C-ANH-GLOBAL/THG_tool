// Domain: outbound × coordination (cross-domain integration; see DOMAINS.md)
package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

func newLedgerTestStore(t *testing.T) *Store {
	return newSharedStore(t, "ledger.db")
}

// Two different FB accounts may queue a comment on the SAME post — that is
// the coordinated-amplification case the Coordination Plane explicitly enables.
// Prior to PR-1 the org-level UNIQUE index blocked the 2nd account.
func TestQueueOutbound_PerAccountAmplificationAllowed(t *testing.T) {
	db := newLedgerTestStore(t)
	postURL := "https://facebook.com/groups/1/posts/100"

	alice := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "Alice's take",
	}
	resAlice, err := db.Outbound().Queue(alice, 24*time.Hour)
	if err != nil {
		t.Fatalf("alice queue: %v", err)
	}
	if !resAlice.Decision.Allowed || resAlice.ID == 0 {
		t.Fatalf("alice should queue: %+v", resAlice.Decision)
	}

	bob := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 20, TargetURL: postURL, Content: "Bob's take",
	}
	resBob, err := db.Outbound().Queue(bob, 24*time.Hour)
	if err != nil {
		t.Fatalf("bob queue: %v", err)
	}
	if !resBob.Decision.Allowed || resBob.ID == 0 {
		t.Fatalf("bob must be allowed (cross-account amplification): %+v", resBob.Decision)
	}
	if resAlice.ID == resBob.ID {
		t.Fatalf("alice and bob got the same row id")
	}
}

// SAME account queueing a 2nd comment on the SAME post within cooldown must
// be blocked — that is the per-account duplicate case.
func TestQueueOutbound_SameAccountDuplicateBlocked(t *testing.T) {
	db := newLedgerTestStore(t)
	postURL := "https://facebook.com/groups/1/posts/101"

	first := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "first",
	}
	if _, err := db.Outbound().Queue(first, 24*time.Hour); err != nil {
		t.Fatalf("first: %v", err)
	}

	second := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "second",
	}
	res, err := db.Outbound().Queue(second, 24*time.Hour)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatalf("same-account duplicate must be blocked")
	}
	if res.Decision.Reason != "duplicate_outbound_per_account" {
		t.Errorf("Reason = %q, want duplicate_outbound_per_account", res.Decision.Reason)
	}
}

// Inbox MUST remain cross-account-strict — 3 staff messaging the same lead in
// the same window is the spam-cluster case.
func TestQueueOutbound_InboxIsCrossAccountStrict(t *testing.T) {
	db := newLedgerTestStore(t)
	leadProfile := "https://facebook.com/profile.php?id=lead_42"

	alice := &models.OutboundMessage{
		OrgID: 1, Type: "inbox", Platform: "facebook",
		AccountID: 10, TargetURL: leadProfile, Content: "Hi from Alice",
	}
	if _, err := db.Outbound().Queue(alice, 24*time.Hour); err != nil {
		t.Fatalf("alice inbox: %v", err)
	}

	bob := &models.OutboundMessage{
		OrgID: 1, Type: "inbox", Platform: "facebook",
		AccountID: 20, TargetURL: leadProfile, Content: "Hi from Bob",
	}
	res, err := db.Outbound().Queue(bob, 24*time.Hour)
	if err != nil {
		t.Fatalf("bob inbox: %v", err)
	}
	if res.Decision.Allowed {
		t.Fatalf("inbox cross-account must be blocked (spam-cluster prevention)")
	}
	if res.Decision.Reason != "duplicate_outbound_target" {
		t.Errorf("Reason = %q, want duplicate_outbound_target (cross-account)", res.Decision.Reason)
	}
}

// Every successful QueueOutboundForOrg call lands a ledger row.
func TestQueueOutbound_RecordsLedger(t *testing.T) {
	db := newLedgerTestStore(t)
	postURL := "https://facebook.com/groups/1/posts/102"

	msg := &models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: postURL, Content: "x",
	}
	res, err := db.Outbound().Queue(msg, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !res.Decision.Allowed {
		t.Fatalf("queue blocked: %+v", res.Decision)
	}

	entries, err := db.Coordination().ListActionLedger(context.Background(), 1, "comment", postURL, time.Time{}, 10)
	if err != nil {
		t.Fatalf("ListActionLedger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.AccountID != 10 || e.OutboundID != res.ID || e.Outcome != coordination.LedgerOutcomeQueued {
		t.Errorf("ledger entry mismatch: %+v", e)
	}
	if e.TargetType != "post" {
		t.Errorf("TargetType = %q, want post (derived from action_type=comment)", e.TargetType)
	}
	if e.CooldownUntil.IsZero() {
		t.Errorf("cooldown_until must be set when cooldown > 0")
	}
}

// Standalone RecordActionLedger + MarkActionLedgerOutcomeByOutbound round-trip.
func TestActionLedger_RecordAndMarkOutcome(t *testing.T) {
	db := newLedgerTestStore(t)
	ctx := context.Background()

	const outboundID = int64(42)
	id, err := db.Coordination().RecordActionLedger(ctx, coordination.ActionLedgerEntry{
		OrgID: 1, ActionType: "group_post",
		TargetURL:  "https://facebook.com/groups/999",
		AccountID:  7,
		OutboundID: outboundID,
	})
	if err != nil {
		t.Fatalf("RecordActionLedger: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, outboundID, coordination.LedgerOutcomeSucceeded, "sent ok"); err != nil {
		t.Fatalf("MarkActionLedgerOutcomeByOutbound: %v", err)
	}
	entries, _ := db.Coordination().ListActionLedger(ctx, 1, "group_post", "https://facebook.com/groups/999", time.Time{}, 10)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Outcome != coordination.LedgerOutcomeSucceeded || !strings.Contains(entries[0].Reason, "sent ok") {
		t.Errorf("outcome/reason not updated: %+v", entries[0])
	}
	if entries[0].TargetType != "group" {
		t.Errorf("TargetType = %q, want group (derived from action_type=group_post)", entries[0].TargetType)
	}
}

// TestTargetTypeFromAction moved to
// internal/store/coordination/action_ledger_internal_test.go (package
// coordination) so the unexported helper stays reachable. Phase 5B.
