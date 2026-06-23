package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Characterization tests for leadsFromActionArgs — the FB-specific "action args →
// lead(s)" seam that the Phase C cut separates from the vertical-neutral queue/claim/
// ledger core (REFACTOR_ROADMAP Phase C; MODULE_BOUNDARIES outbound vs services/facebook).
// These pin CURRENT behavior so the eventual split can be proven behavior-preserving.
// They assert what the code does today, NOT what it "should" do.

func newArgsStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "leads_args_test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// lead_id path: resolves the ONE existing org-scoped lead and returns it as-is.
func TestLeadsFromActionArgs_LeadIDPath(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	const orgID = int64(5)
	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post", SourceURL: "https://www.facebook.com/groups/1/posts/2/",
		Platform: models.PlatformFacebook, Author: "An", Content: "real lead content", Score: models.LeadHot,
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}

	leads, err := leadsFromActionArgs(ctx, db, orgID, "comment", map[string]any{"lead_id": id})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 1 || leads[0].ID != id || leads[0].Content != "real lead content" {
		t.Fatalf("lead_id path must return the one real lead, got %+v", leads)
	}
}

// lead_id path is org-scoped: a lead_id from another org returns no leads (nil, nil),
// NOT a cross-tenant read. GetLeadByID filters by org_id, so a wrong-org id is "not found".
func TestLeadsFromActionArgs_LeadIDTenantScoped(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	id, err := db.Leads().InsertLead(&models.Lead{
		OrgID: 5, SourceType: "post", SourceURL: "https://www.facebook.com/groups/1/posts/2/",
		Platform: models.PlatformFacebook, Content: "org5 lead", Score: models.LeadHot,
	})
	if err != nil {
		t.Fatalf("InsertLead: %v", err)
	}
	// Same lead_id, but a DIFFERENT org → not found → no leads (no cross-tenant leak).
	leads, err := leadsFromActionArgs(ctx, db, 6, "comment", map[string]any{"lead_id": id})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("lead_id from another org must yield no leads, got %+v", leads)
	}
}

// comment msgType with post_url builds a synthetic prompt_target lead carrying the
// routing contract fields the FB target-URL resolver later consumes.
func TestLeadsFromActionArgs_CommentSyntheticFromPostURL(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	const orgID = int64(5)
	const post = "https://www.facebook.com/groups/1/posts/2/"
	leads, err := leadsFromActionArgs(ctx, db, orgID, "comment", map[string]any{
		"post_url":    post,
		"target_name": "Author Name",
		"author_url":  "https://www.facebook.com/author",
		"context":     "lead context text",
	})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one synthetic lead, got %d", len(leads))
	}
	got := leads[0]
	want := models.Lead{
		OrgID:      orgID,
		SourceURL:  post,
		Author:     "Author Name",
		AuthorURL:  "https://www.facebook.com/author",
		Content:    "lead context text",
		Score:      models.LeadHot,
		Platform:   models.PlatformFacebook,
		SourceType: "prompt_target",
	}
	if got != want {
		t.Errorf("synthetic comment lead = %+v, want %+v", got, want)
	}
}

// comment msgType prefers post_url over target_url (FirstNonEmpty order).
func TestLeadsFromActionArgs_CommentPrefersPostURL(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	leads, err := leadsFromActionArgs(ctx, db, 5, "comment", map[string]any{
		"post_url":   "https://www.facebook.com/post_url",
		"target_url": "https://www.facebook.com/target_url",
	})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 1 || leads[0].SourceURL != "https://www.facebook.com/post_url" {
		t.Fatalf("comment must prefer post_url, got %+v", leads)
	}
}

// comment msgType falls back to target_url when post_url is empty.
func TestLeadsFromActionArgs_CommentFallsBackToTargetURL(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	leads, err := leadsFromActionArgs(ctx, db, 5, "comment", map[string]any{
		"target_url": "https://www.facebook.com/target_url",
	})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 1 || leads[0].SourceURL != "https://www.facebook.com/target_url" {
		t.Fatalf("comment must fall back to target_url, got %+v", leads)
	}
}

// non-comment msgType with target_url puts the target into AuthorURL (NOT SourceURL),
// because the inbox/post target is the participant, per the routing contract.
func TestLeadsFromActionArgs_NonCommentSyntheticUsesAuthorURL(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	const orgID = int64(5)
	leads, err := leadsFromActionArgs(ctx, db, orgID, "inbox", map[string]any{
		"target_url":  "https://www.facebook.com/participant",
		"target_name": "Participant",
		"context":     "ctx",
	})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 1 {
		t.Fatalf("expected one synthetic lead, got %d", len(leads))
	}
	got := leads[0]
	want := models.Lead{
		OrgID:      orgID,
		AuthorURL:  "https://www.facebook.com/participant",
		Author:     "Participant",
		Content:    "ctx",
		Score:      models.LeadHot,
		Platform:   models.PlatformFacebook,
		SourceType: "prompt_target",
	}
	if got != want {
		t.Errorf("synthetic non-comment lead = %+v, want %+v", got, want)
	}
}

// comment with NO post_url/target_url and no lead_id falls through to the WorkQueue
// (empty store → empty result, the "no eligible lead" path).
func TestLeadsFromActionArgs_NoTargetFallsThroughToWorkQueue(t *testing.T) {
	ctx := context.Background()
	db := newArgsStore(t)
	leads, err := leadsFromActionArgs(ctx, db, 5, "comment", map[string]any{})
	if err != nil {
		t.Fatalf("leadsFromActionArgs: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("no target + empty work queue must yield no leads, got %+v", leads)
	}
}
