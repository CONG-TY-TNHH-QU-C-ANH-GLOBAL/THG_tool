package store

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func newEngagementTestStore(t *testing.T) *Store {
	return newSharedStore(t, "engagement.db")
}

// Untouched lead: no action_ledger rows, no thread → badge=priority,
// entries empty.
func TestLeadEngagement_UntouchedIsPriority(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	leadID := seedLead(t, db, 1, "https://facebook.com/post/A", "https://facebook.com/profile/L", "")

	state, err := db.GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if state.Badge != models.LeadBadgePriority {
		t.Errorf("badge = %s, want priority", state.Badge)
	}
	if len(state.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(state.Entries))
	}
}

// One staff (Alice) queues a comment on the lead's source_url. The
// projection must surface that ledger row with user_name resolved
// through accounts.assigned_user_id → users.name.
func TestLeadEngagement_ResolvesUserViaAccountAssignment(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	aliceUserID := seedUser(t, db, 1, "Alice")
	aliceAccID := seedAccount(t, db, 1, "Alice FB", aliceUserID)

	leadID := seedLead(t, db, 1, "https://facebook.com/post/B", "https://facebook.com/profile/B", "")

	// Use the canonical queue path so the ledger row is real.
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: aliceAccID, TargetURL: "https://facebook.com/post/B", Content: "alice was here",
	}, false, 24*time.Hour)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if !res.Decision.Allowed {
		t.Fatalf("queue should allow: %+v", res.Decision)
	}

	state, err := db.GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(state.Entries))
	}
	got := state.Entries[0]
	if got.UserID != aliceUserID {
		t.Errorf("user_id = %d, want %d", got.UserID, aliceUserID)
	}
	if got.UserName != "Alice" {
		t.Errorf("user_name = %q, want %q", got.UserName, "Alice")
	}
	if got.Action != "comment" {
		t.Errorf("action = %q, want comment", got.Action)
	}
	// Just queued → badge=protected (within 15min window).
	if state.Badge != models.LeadBadgeProtected {
		t.Errorf("badge = %s, want protected", state.Badge)
	}
	if state.LastEngagedBy != "Alice" {
		t.Errorf("last_engaged_by = %q, want Alice", state.LastEngagedBy)
	}
}

// Lead URL surfaces: action against the lead's author_url (inbox case)
// must also be projected, not just source_url.
func TestLeadEngagement_ProjectsAcrossAllLeadURLs(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	bobUserID := seedUser(t, db, 1, "Bob")
	bobAccID := seedAccount(t, db, 1, "Bob FB", bobUserID)

	leadID := seedLead(t, db, 1,
		"https://facebook.com/post/C",
		"https://facebook.com/profile/C",
		"https://facebook.com/post/C?cmt=42")

	// Inbox on the author_url.
	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "inbox", Platform: "facebook",
		AccountID: bobAccID, TargetURL: "https://facebook.com/profile/C", Content: "hi",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("inbox queue: %v", err)
	}
	// Comment on the secondary_url (rare but supported).
	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: bobAccID, TargetURL: "https://facebook.com/post/C?cmt=42", Content: "reply",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("comment queue: %v", err)
	}

	state, err := db.GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 2 {
		t.Fatalf("expected 2 entries across author_url + secondary_url, got %d", len(state.Entries))
	}
}

// Org isolation: a ledger entry against an identical target_url in a
// different org must NOT be projected onto the lead. Battlefield is
// shared WITHIN an org, never across orgs.
func TestLeadEngagement_OrgScopedProjection(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	aliceUserID := seedUser(t, db, 1, "Alice")
	aliceAccID := seedAccount(t, db, 1, "Alice FB", aliceUserID)
	otherUserID := seedUser(t, db, 2, "Outsider")
	otherAccID := seedAccount(t, db, 2, "Other FB", otherUserID)

	leadID := seedLead(t, db, 1, "https://facebook.com/post/D", "https://facebook.com/profile/D", "")

	// Org 2 engages the same URL — must NOT bleed into org 1's view.
	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 2, Type: "comment", Platform: "facebook",
		AccountID: otherAccID, TargetURL: "https://facebook.com/post/D", Content: "other org",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("other org queue: %v", err)
	}
	// Org 1 also engages.
	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: aliceAccID, TargetURL: "https://facebook.com/post/D", Content: "alice org1",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("alice queue: %v", err)
	}

	state, err := db.GetLeadEngagement(ctx, 1, leadID)
	if err != nil {
		t.Fatalf("GetLeadEngagement: %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("expected 1 entry in org 1, got %d", len(state.Entries))
	}
	if state.Entries[0].UserName != "Alice" {
		t.Errorf("expected Alice's entry, got %q", state.Entries[0].UserName)
	}
}

// Batch endpoint: must return distinct engagement states keyed by
// lead_id, without leaking entries between leads.
func TestLeadEngagement_Batch(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	aliceUserID := seedUser(t, db, 1, "Alice")
	aliceAccID := seedAccount(t, db, 1, "Alice FB", aliceUserID)

	leadAlpha := seedLead(t, db, 1, "https://facebook.com/post/Alpha", "https://facebook.com/profile/Alpha", "")
	leadBeta := seedLead(t, db, 1, "https://facebook.com/post/Beta", "https://facebook.com/profile/Beta", "")
	leadGamma := seedLead(t, db, 1, "https://facebook.com/post/Gamma", "https://facebook.com/profile/Gamma", "")

	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: aliceAccID, TargetURL: "https://facebook.com/post/Alpha", Content: "x",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("queue alpha: %v", err)
	}
	// Beta has TWO actions; Gamma has zero.
	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "inbox", Platform: "facebook",
		AccountID: aliceAccID, TargetURL: "https://facebook.com/profile/Beta", Content: "y",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("queue beta inbox: %v", err)
	}

	got, err := db.GetLeadEngagementsBatch(ctx, 1, []int64{leadAlpha, leadBeta, leadGamma})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 states, got %d", len(got))
	}
	if len(got[leadAlpha].Entries) != 1 || got[leadAlpha].Badge != models.LeadBadgeProtected {
		t.Errorf("alpha state wrong: %+v", got[leadAlpha])
	}
	if len(got[leadBeta].Entries) != 1 || got[leadBeta].Badge != models.LeadBadgeProtected {
		t.Errorf("beta state wrong: %+v", got[leadBeta])
	}
	if len(got[leadGamma].Entries) != 0 || got[leadGamma].Badge != models.LeadBadgePriority {
		t.Errorf("gamma must be untouched priority, got %+v", got[leadGamma])
	}
}

// Different leads must not bleed entries when their URLs are similar but
// not equal.
func TestLeadEngagement_BatchNoCrossLeadBleed(t *testing.T) {
	db := newEngagementTestStore(t)
	ctx := context.Background()

	aliceUserID := seedUser(t, db, 1, "Alice")
	aliceAccID := seedAccount(t, db, 1, "Alice FB", aliceUserID)

	leadOne := seedLead(t, db, 1, "https://facebook.com/post/100", "https://facebook.com/profile/100", "")
	leadTwo := seedLead(t, db, 1, "https://facebook.com/post/200", "https://facebook.com/profile/200", "")

	if _, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: aliceAccID, TargetURL: "https://facebook.com/post/100", Content: "for 100",
	}, false, 24*time.Hour); err != nil {
		t.Fatalf("queue 100: %v", err)
	}

	states, err := db.GetLeadEngagementsBatch(ctx, 1, []int64{leadOne, leadTwo})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(states[leadOne].Entries) != 1 {
		t.Errorf("lead 100 should have 1 entry, got %d", len(states[leadOne].Entries))
	}
	if len(states[leadTwo].Entries) != 0 {
		t.Errorf("lead 200 must NOT inherit lead 100's entry, got %d", len(states[leadTwo].Entries))
	}
}

// Helpers — minimal seeders avoiding the full Account/User insert path.

func seedUser(t *testing.T, db *Store, orgID int64, name string) int64 {
	t.Helper()
	res, err := db.db.Exec(
		`INSERT INTO users (email, name, password_hash, role, active, org_id)
		 VALUES (?, ?, '', 'sales', 1, ?)`,
		name+"@example.test", name, orgID,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedAccount(t *testing.T, db *Store, orgID int64, name string, userID int64) int64 {
	t.Helper()
	res, err := db.db.Exec(
		`INSERT INTO accounts (platform, name, status, org_id, assigned_user_id)
		 VALUES ('facebook', ?, 'active', ?, ?)`,
		name, orgID, userID,
	)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedLead(t *testing.T, db *Store, orgID int64, sourceURL, authorURL, secondaryURL string) int64 {
	t.Helper()
	res, err := db.db.Exec(
		`INSERT INTO leads (org_id, source_type, source_id, source_url, author_url, secondary_url,
		                    platform, content, score, niche)
		 VALUES (?, 'post', 0, ?, ?, ?, 'facebook', '', 'cold', 'logistics')`,
		orgID, sourceURL, authorURL, secondaryURL,
	)
	if err != nil {
		t.Fatalf("seed lead: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
