package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Lead Facebook Interaction Attribution: the engagement projection must surface
// WHICH Facebook account commented a shared lead — account_id + fb_display_name +
// actor verdict — attributed to the IMMUTABLE created_by initiator (NOT the
// account's mutable assigned_user_id). Observability only; the lead stays shared.
func TestLeadEngagement_FacebookActorAttribution(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	alice := seedUser(t, db, 1, "Alice")
	bob := seedUser(t, db, 1, "Bob")
	// Account is ASSIGNED to Bob, but Alice initiates the action.
	accID := seedAccount(t, db, 1, "Sales FB", bob)
	if _, err := db.db.Exec(`UPDATE accounts SET fb_display_name=?, fb_profile_url=? WHERE id=?`,
		"Anh Đức", "https://facebook.com/anhduc", accID); err != nil {
		t.Fatalf("set fb identity: %v", err)
	}
	if err := db.Coordination().RecordAccountActorVerdict(ctx, 1, accID, models.ActorVerdictVerified, "100", "", false); err != nil {
		t.Fatalf("record verdict: %v", err)
	}

	leadID := seedLead(t, db, 1, "https://facebook.com/post/X", "https://facebook.com/profile/X", "")
	res, err := db.Outbound().Queue(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: accID, CreatedBy: alice, TargetURL: "https://facebook.com/post/X", Content: "hi",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, res.ID, "succeeded", "verified by test"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	state, err := db.Leads().GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(state.Entries))
	}
	e := state.Entries[0]
	if e.AccountID != accID {
		t.Errorf("account_id = %d, want %d", e.AccountID, accID)
	}
	if e.FBDisplayName != "Anh Đức" {
		t.Errorf("fb_display_name = %q, want 'Anh Đức'", e.FBDisplayName)
	}
	if e.FBProfileURL != "https://facebook.com/anhduc" {
		t.Errorf("fb_profile_url = %q", e.FBProfileURL)
	}
	if e.ActorVerdict != models.ActorVerdictVerified {
		t.Errorf("actor_verdict = %q, want verified", e.ActorVerdict)
	}
	if e.Channel != "facebook" {
		t.Errorf("channel = %q, want facebook", e.Channel)
	}
	// Attribution rides created_by (Alice), NOT the account's assigned user (Bob).
	if e.UserID != alice || e.UserName != "Alice" {
		t.Errorf("attribution = %d/%q, want Alice (created_by, not assigned Bob)", e.UserID, e.UserName)
	}
}

// Optimistic Success Semantics Hardening: a submitted-but-unverified comment
// (action_ledger.outcome='submitted_unverified') is NOT a verified touch — the Lead
// Engagement projection (WHERE outcome='succeeded') must exclude it entirely, so the
// Lead tab never shows it as "Đã comment".
func TestLeadEngagement_SubmittedUnverifiedExcluded(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	user := seedUser(t, db, 1, "Alice")
	accID := seedAccount(t, db, 1, "Sales FB", user)
	leadID := seedLead(t, db, 1, "https://facebook.com/post/Z", "https://facebook.com/profile/Z", "")
	res, err := db.Outbound().Queue(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: accID, CreatedBy: user, TargetURL: "https://facebook.com/post/Z", Content: "submitted",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	// Finalize as submitted_unverified (what LedgerOutcomeAlias(optimistic) writes).
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, res.ID, models.LedgerOutcomeSubmittedUnverified, "optimistic"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	state, err := db.Leads().GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("submitted_unverified must NOT appear as a verified interaction; got %d entries", len(state.Entries))
	}
	if state.Badge == models.LeadBadgeProtected || state.Badge == models.LeadBadgeVisible {
		t.Errorf("submitted_unverified must not promote the engagement badge; got %s", state.Badge)
	}
}

// created_by=0 (system / agent auto-comment) must project gracefully: account is
// still surfaced, initiator is empty (not a crash, not a fake user).
func TestLeadEngagement_FacebookActor_SystemCreatedByGraceful(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	accID := seedAccount(t, db, 1, "Agent FB", 0)
	leadID := seedLead(t, db, 1, "https://facebook.com/post/Y", "https://facebook.com/profile/Y", "")
	res, err := db.Outbound().Queue(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: accID, CreatedBy: 0, TargetURL: "https://facebook.com/post/Y", Content: "agent auto",
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, 1, res.ID, "succeeded", "ok"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	state, err := db.Leads().GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(state.Entries))
	}
	e := state.Entries[0]
	if e.UserID != 0 || e.UserName != "" {
		t.Errorf("system created_by should project 0/empty, got %d/%q", e.UserID, e.UserName)
	}
	if e.AccountID != accID || e.Channel != "facebook" {
		t.Errorf("account should still project: account_id=%d channel=%q", e.AccountID, e.Channel)
	}
}
