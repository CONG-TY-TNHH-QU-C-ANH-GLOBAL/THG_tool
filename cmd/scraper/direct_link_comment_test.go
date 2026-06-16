package main

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

// resolveDirectCommentURL is pure — assert every §7 URL-layer state.
func TestResolveDirectCommentURL(t *testing.T) {
	const post = "https://www.facebook.com/groups/123/posts/456/"
	cases := []struct {
		name, prompt, postArg string
		wantBlocked           bool
		wantContains          string // substring of message (blocked) or canonical (ok)
	}{
		{"no url asks for link", "comment bài này", "", true, "gửi giúp tôi link"},
		{"two urls one only", "comment " + post + " và " + "https://www.facebook.com/groups/9/posts/9/", "", true, "chỉ gửi một link"},
		{"unsupported group shell", "comment bài này https://www.facebook.com/groups/123", "", true, "chưa được hỗ trợ"},
		{"valid normalizes to canonical", "comment bài này " + post, post, false, "https://www.facebook.com/groups/123/permalink/456/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := resolveDirectCommentURL(c.prompt, c.postArg)
			if r.blocked != c.wantBlocked {
				t.Fatalf("blocked=%v want %v (msg=%q canonical=%q)", r.blocked, c.wantBlocked, r.message, r.canonical)
			}
			hay := r.message
			if !r.blocked {
				hay = r.canonical
			}
			if !strings.Contains(hay, c.wantContains) {
				t.Errorf("got %q, want contains %q", hay, c.wantContains)
			}
		})
	}
}

func bootstrapDirectStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// commentSinglePost: the P1.3D live-identity account guard fires FIRST. With no live Chrome
// connector, BOTH the unknown-post and existing-post paths fail closed on the account
// identity — proving no workflow/import/outbound is created when the live account is
// unknown (the wrong-account hazard). Downstream delegation to the shared gates (when an
// account IS resolved) is covered by direct_post_intake_test.go + queueLeadOutreach tests.
func TestCommentSinglePost_Delegation(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapDirectStore, "direct_link_comment")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)
	msgGen := ai.NewMessageGenerator("", "") // not Available — generation is never reached here

	if _, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "acc-a", Status: models.AccountActive,
	}); err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	post := "https://www.facebook.com/groups/123/posts/456/"
	if _, err := db.Leads().InsertLead(&models.Lead{
		OrgID: orgID, SourceType: "post", SourceURL: post, PostFBID: "456", GroupFBID: "123",
		Platform: models.PlatformFacebook, Author: "An", Content: "ai làm fulfill US", Score: models.LeadHot,
	}); err != nil {
		t.Fatalf("InsertLead: %v", err)
	}

	// Proven requester (user_id) but NO live Chrome connector → the account guard blocks BEFORE
	// the lead lookup, for both an unknown post (no workflow/import created) ...
	unknown, err := commentSinglePost(ctx, db, msgGen, map[string]any{
		"org_id": orgID, "user_id": int64(7),
		"nl_prompt": "comment bài này https://www.facebook.com/groups/123/posts/999/",
	}, nil, nil)
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}
	if !strings.Contains(unknown, "đăng nhập trong Chrome") {
		t.Errorf("unknown post must fail closed on the live-account guard, got %q", unknown)
	}

	// ... and an existing post (no outbound queued) — fail closed on the account identity.
	found, err := commentSinglePost(ctx, db, msgGen, map[string]any{
		"org_id": orgID, "user_id": int64(7),
		"nl_prompt": "comment bài này " + post,
	}, nil, nil)
	if err != nil {
		t.Fatalf("found: %v", err)
	}
	if !strings.Contains(found, "đăng nhập trong Chrome") {
		t.Errorf("existing post must fail closed on the live-account guard, got %q", found)
	}
}
