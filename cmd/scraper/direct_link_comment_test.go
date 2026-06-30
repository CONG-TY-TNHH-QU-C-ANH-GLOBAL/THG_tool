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

// ResolveCommentURL moved to internal/directpost (pure validation leaf); its §7
// URL-state assertions live in directpost/url_resolve_test.go.

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
