package main

import "testing"

// P1.3E: the single-post import args carry the direct-post target identity so the extension can
// extract ONLY the requested post (and submitOpenCrawl forwards it into extras.direct_post_target).
func TestDirectPostImportArgs_CarriesTargetIdentity(t *testing.T) {
	args := directPostImportArgs(directPostCommentInput{
		OrgID: 5, RequestedByUserID: 7, AccountID: 50, UserRole: "sales",
		CanonicalPostURL: intakeCanonical, PostFBID: intakePostFBID, GroupRef: "ship.viet.my", Prompt: "comment",
	})
	if got := argString(args, "direct_post_post_fbid"); got != intakePostFBID {
		t.Errorf("direct_post_post_fbid = %q, want %q", got, intakePostFBID)
	}
	if got := argString(args, "direct_post_group_ref"); got != "ship.viet.my" {
		t.Errorf("direct_post_group_ref = %q, want ship.viet.my", got)
	}
	if got := argString(args, "direct_post_canonical"); got != intakeCanonical {
		t.Errorf("direct_post_canonical = %q, want %q", got, intakeCanonical)
	}
	if argInt64(args, "max_items") != 1 {
		t.Errorf("direct-post import must be max_items=1 (never a broad crawl)")
	}
}
