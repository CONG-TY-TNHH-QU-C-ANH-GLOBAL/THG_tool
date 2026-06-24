package facebook

import (
	"context"
	"fmt"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// fakeLeadSource is a store-free LeadSource: LeadByID is keyed by "org:id" (so a
// wrong-org lookup misses, proving LeadsForAction forwards orgID), and
// WorkQueueLeads records the score + pool it was called with.
type fakeLeadSource struct {
	byID       map[string]*models.Lead
	queue      []models.Lead
	gotScore   string
	gotPool    int
	queueCalls int
}

func (f *fakeLeadSource) LeadByID(_ context.Context, orgID, leadID int64) (*models.Lead, error) {
	return f.byID[fmt.Sprintf("%d:%d", orgID, leadID)], nil
}

func (f *fakeLeadSource) WorkQueueLeads(_ context.Context, _ int64, score string, pool int) ([]models.Lead, error) {
	f.queueCalls++
	f.gotScore, f.gotPool = score, pool
	return f.queue, nil
}

func TestRequestedOutreachCount(t *testing.T) {
	cases := []struct{ limit, maxItems, want int }{
		{5, 0, 5},   // explicit limit wins
		{0, 3, 3},   // falls back to max_items
		{0, 7, 7},   // limit zero → max_items
		{0, 0, 25},  // default
		{-2, 0, 25}, // non-positive limit → default (no max_items)
	}
	for _, c := range cases {
		if got := RequestedOutreachCount(c.limit, c.maxItems); got != c.want {
			t.Errorf("RequestedOutreachCount(%d,%d) = %d, want %d", c.limit, c.maxItems, got, c.want)
		}
	}
	// scanPoolFor is max(50, requested*10): a request of 5 scans up to 50; 8 scans 80.
	if got := scanPoolFor(5); got != 50 {
		t.Errorf("scanPoolFor(5) = %d, want 50", got)
	}
	if got := scanPoolFor(8); got != 80 {
		t.Errorf("scanPoolFor(8) = %d, want 80", got)
	}
}

// lead_id path: resolves the ONE existing org-scoped lead and returns it as-is,
// without consulting the work queue.
func TestLeadsForAction_LeadIDPath(t *testing.T) {
	ctx := context.Background()
	src := &fakeLeadSource{byID: map[string]*models.Lead{
		"5:7": {ID: 7, OrgID: 5, Content: "real lead content"},
	}}
	leads, err := LeadsForAction(ctx, src, 5, "comment", LeadSelectionInput{LeadID: 7})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 1 || leads[0].ID != 7 || leads[0].Content != "real lead content" {
		t.Fatalf("lead_id path must return the one real lead, got %+v", leads)
	}
	if src.queueCalls != 0 {
		t.Errorf("lead_id path must NOT consult the work queue, got %d calls", src.queueCalls)
	}
}

// lead_id is org-scoped: LeadsForAction forwards orgID to the port, so a lead_id
// queried under a different org misses (nil) and yields no leads (no cross-tenant).
func TestLeadsForAction_LeadIDForwardsOrg(t *testing.T) {
	ctx := context.Background()
	src := &fakeLeadSource{byID: map[string]*models.Lead{
		"5:7": {ID: 7, OrgID: 5, Content: "org5 lead"},
	}}
	leads, err := LeadsForAction(ctx, src, 6, "comment", LeadSelectionInput{LeadID: 7})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("lead_id under another org must yield no leads, got %+v", leads)
	}
}

// comment + post_url builds a synthetic prompt_target lead carrying the routing
// contract fields; the work queue is not consulted.
func TestLeadsForAction_CommentSyntheticFromPostURL(t *testing.T) {
	ctx := context.Background()
	const orgID = int64(5)
	const post = "https://www.facebook.com/groups/1/posts/2/"
	src := &fakeLeadSource{}
	leads, err := LeadsForAction(ctx, src, orgID, "comment", LeadSelectionInput{
		PostURL:    post,
		TargetName: "Author Name",
		AuthorURL:  "https://www.facebook.com/author",
		Context:    "lead context text",
	})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	want := models.Lead{
		OrgID: orgID, SourceURL: post, Author: "Author Name",
		AuthorURL: "https://www.facebook.com/author", Content: "lead context text",
		Score: models.LeadHot, Platform: models.PlatformFacebook, SourceType: "prompt_target",
	}
	if len(leads) != 1 || leads[0] != want {
		t.Errorf("synthetic comment lead = %+v, want %+v", leads, want)
	}
	if src.queueCalls != 0 {
		t.Errorf("synthetic path must NOT consult the work queue, got %d calls", src.queueCalls)
	}
}

// comment prefers post_url over target_url; empty post_url falls back to target_url.
func TestLeadsForAction_CommentURLPrecedence(t *testing.T) {
	ctx := context.Background()
	prefers, err := LeadsForAction(ctx, &fakeLeadSource{}, 5, "comment", LeadSelectionInput{
		PostURL:   "https://www.facebook.com/post_url",
		TargetURL: "https://www.facebook.com/target_url",
	})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(prefers) != 1 || prefers[0].SourceURL != "https://www.facebook.com/post_url" {
		t.Fatalf("comment must prefer post_url, got %+v", prefers)
	}

	fallback, err := LeadsForAction(ctx, &fakeLeadSource{}, 5, "comment", LeadSelectionInput{
		TargetURL: "https://www.facebook.com/target_url",
	})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(fallback) != 1 || fallback[0].SourceURL != "https://www.facebook.com/target_url" {
		t.Fatalf("comment must fall back to target_url, got %+v", fallback)
	}
}

// non-comment (inbox) target_url goes into AuthorURL (the participant), per the
// routing contract — NOT SourceURL.
func TestLeadsForAction_NonCommentUsesAuthorURL(t *testing.T) {
	ctx := context.Background()
	const orgID = int64(5)
	leads, err := LeadsForAction(ctx, &fakeLeadSource{}, orgID, "inbox", LeadSelectionInput{
		TargetURL:  "https://www.facebook.com/participant",
		TargetName: "Participant",
		Context:    "ctx",
	})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	want := models.Lead{
		OrgID: orgID, AuthorURL: "https://www.facebook.com/participant", Author: "Participant",
		Content: "ctx", Score: models.LeadHot, Platform: models.PlatformFacebook, SourceType: "prompt_target",
	}
	if len(leads) != 1 || leads[0] != want {
		t.Errorf("synthetic non-comment lead = %+v, want %+v", leads, want)
	}
}

// No lead_id and no prompt target → work-queue fallback. Comment passes the score
// filter through and sizes the pool from limit; inbox with no score defaults "hot".
func TestLeadsForAction_WorkQueueFallback(t *testing.T) {
	ctx := context.Background()

	// comment: empty work queue → no leads; pool = scanPoolFor(RequestedOutreachCount(5,0)) = 50.
	src := &fakeLeadSource{}
	leads, err := LeadsForAction(ctx, src, 5, "comment", LeadSelectionInput{ScoreFilter: "hot", Limit: 5})
	if err != nil {
		t.Fatalf("LeadsForAction: %v", err)
	}
	if len(leads) != 0 {
		t.Fatalf("empty work queue must yield no leads, got %+v", leads)
	}
	if src.queueCalls != 1 || src.gotScore != "hot" || src.gotPool != 50 {
		t.Errorf("work queue call: calls=%d score=%q pool=%d, want 1/\"hot\"/50", src.queueCalls, src.gotScore, src.gotPool)
	}

	// inbox with no explicit score defaults to the "hot" band.
	inbox := &fakeLeadSource{}
	if _, err := LeadsForAction(ctx, inbox, 5, "inbox", LeadSelectionInput{}); err != nil {
		t.Fatalf("LeadsForAction(inbox): %v", err)
	}
	if inbox.gotScore != "hot" {
		t.Errorf("inbox default score = %q, want \"hot\"", inbox.gotScore)
	}
}
