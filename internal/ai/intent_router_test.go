package ai

import "testing"

// Characterization of the CURRENT (intended) deterministic routing after the
// intent-layer split — locks behavior before any future typo/multilingual NLU.
// (Single-post + lookalike + crawl-verb edges live in agent_router_single_post_test.go.)
func TestDeterministicFacebookAction_Characterization(t *testing.T) {
	const post = "https://www.facebook.com/groups/123/posts/456/"
	const group = "https://www.facebook.com/groups/123"
	cases := []struct {
		name, prompt, want string
	}{
		{"single post: lead này + url", "comment lead này " + post, "comment_single_post"},
		{"single post: bài này no url (ask-for-link)", "comment bài này", "comment_single_post"},
		{"bulk: tất cả leads", "comment tất cả leads", "comment_all_leads"},
		{"bulk: các lead", "comment cho các lead", "comment_all_leads"},
		{"scrape comments: crawl verb + comment + post url", "cào comment " + post, "scrape_comments"},
		{"scrape group: cào lead + group url", "cào lead " + group, "scrape_group"},
		{"search groups: crawl verb, no url", "cào lead thời trang nữ cao cấp", "search_groups"},
		{"create job post: đăng bài", "đăng bài tuyển dụng nhân viên kho", "create_job_post"},
		{"inbox bulk: inbox tất cả leads", "inbox tất cả leads", "inbox_all_leads"},
		{"no match: chit-chat", "xin chào bạn khỏe không", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, _, ok := deterministicFacebookAction(c.prompt, 5, 0)
			if c.want == "" {
				if ok {
					t.Fatalf("expected no match, got %q", action)
				}
				return
			}
			if !ok || action != c.want {
				t.Fatalf("action = %q (ok=%v), want %q", action, ok, c.want)
			}
		})
	}
}

// Multiple FB URLs: the router still classifies a comment-on-a-post (first URL);
// the "chỉ gửi một link" one-link-only guard lives in the orchestrator
// (cmd/scraper resolveDirectCommentURL), tested there.
func TestDeterministicFacebookAction_MultipleURLsStillSinglePost(t *testing.T) {
	two := "comment bài này https://www.facebook.com/groups/1/posts/2/ và https://www.facebook.com/groups/3/posts/4/"
	if action, _, ok := deterministicFacebookAction(two, 5, 0); !ok || action != "comment_single_post" {
		t.Fatalf("two URLs + comment should route single-post (orchestrator enforces one-link), got %q", action)
	}
}

// RouteDecisionFor is the safe observability view — secret-free, populated flags.
func TestRouteDecisionFor(t *testing.T) {
	rd := RouteDecisionFor("comment bài này https://www.facebook.com/groups/1/posts/2/")
	if rd.Action != "comment_single_post" || rd.Confidence != ConfidenceHigh {
		t.Fatalf("route decision = %+v", rd)
	}
	if rd.URLCount != 1 || rd.HasCrawlVerb {
		t.Errorf("expected url_count=1, has_crawl_verb=false, got %+v", rd)
	}
	if miss := RouteDecisionFor("xin chào"); miss.Action != "" || miss.Confidence != ConfidenceLow {
		t.Errorf("no-match should be low confidence + empty action, got %+v", miss)
	}
}

// Branch-aware bulk-scope observability: the debug flags must reflect the branch
// that actually routed (inbox bulk accepts a bare "lead"; comment bulk does not).
// Routing itself is asserted unchanged.
func TestRouteDecisionFor_BranchAwareBulkScope(t *testing.T) {
	// inbox bulk via a bare singular "lead" → inbox_all_leads; inbox bulk flag set,
	// comment bulk flag NOT set (this was the mislabel the fix targets).
	in := RouteDecisionFor("inbox lead này")
	if in.Action != "inbox_all_leads" {
		t.Fatalf("routing changed: inbox lead này → %q (want inbox_all_leads)", in.Action)
	}
	if !in.HasInboxBulkScope || in.HasCommentBulkScope || !in.HasBulkScope {
		t.Errorf("inbox-bulk flags wrong: %+v", in)
	}

	// comment bulk → comment_all_leads; comment bulk flag set, inbox-only bare-lead
	// not the trigger here ("tất cả leads" is in BOTH sets, so inbox flag is also true —
	// the point is comment bulk is correctly true and routing is comment_all_leads).
	cm := RouteDecisionFor("comment tất cả leads")
	if cm.Action != "comment_all_leads" {
		t.Fatalf("routing changed: comment tất cả leads → %q", cm.Action)
	}
	if !cm.HasCommentBulkScope || !cm.HasBulkScope {
		t.Errorf("comment-bulk flags wrong: %+v", cm)
	}

	// direct single-post comment → comment_single_post; NO bulk scope of either kind.
	sp := RouteDecisionFor("comment bài này https://www.facebook.com/groups/1/posts/2/")
	if sp.Action != "comment_single_post" {
		t.Fatalf("routing changed: single post → %q", sp.Action)
	}
	if sp.HasBulkScope || sp.HasCommentBulkScope || sp.HasInboxBulkScope {
		t.Errorf("single-post must have no bulk scope, got %+v", sp)
	}
}

// TODO(nlu): when the typo-tolerant / multilingual NLU lands, add characterization
// cases here (e.g. "comment bài nay" without diacritic, English "comment this post
// <url>"). They are intentionally NOT added as failing tests yet — this PR is the
// structural boundary only.
