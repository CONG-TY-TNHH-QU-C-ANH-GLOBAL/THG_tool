package copilot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/store"
)

// P0 production bug: a direct "comment THIS post <url>" prompt must route
// deterministically to comment_single_post (before the brain), for numeric AND
// vanity group permalinks. Pins the EXACT production prompt.
func TestP0_DirectCommentRoutesSinglePost(t *testing.T) {
	const vanity = "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/"
	const numeric = "https://www.facebook.com/groups/123456789/permalink/4504452536547584/"
	const posts = "https://www.facebook.com/groups/123/posts/456/"
	cases := []struct{ name, prompt, url string }{
		// A — the exact production prompt that returned generic text.
		{"A production prompt (vanity)", "Comment bài này cho tôi " + vanity, vanity},
		// B — numeric group permalink.
		{"B numeric group permalink", "comment lead này " + numeric, numeric},
		// C — vanity group permalink.
		{"C vanity group permalink", "comment lead này " + vanity, vanity},
		// multilingual exact verb (already supported — pinned, NOT new P2 NLU).
		{"bình luận bài này", "bình luận bài này " + posts, posts},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, args, ok := deterministicFacebookAction(c.prompt, 5, 0)
			if !ok || action != "comment_single_post" {
				t.Fatalf("action = %q (ok=%v), want comment_single_post (not brain/crawler/comment_all_leads)", action, ok)
			}
			if got := argStringFromMap(args, "post_url"); got != c.url {
				t.Errorf("post_url = %q, want %q", got, c.url)
			}
		})
	}
}

// The early-bypass predicate: true only for a scope-confirmed direct-post comment;
// crawl/bulk/lookalike/no-url must stay false (→ fall through to the brain).
func TestP0_PromptIsDirectPostComment(t *testing.T) {
	const post = "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/"
	cases := []struct {
		name, prompt string
		want         bool
	}{
		{"A production prompt", "Comment bài này cho tôi " + post, true},
		{"B comment lead này", "comment lead này " + post, true},
		{"D crawl verb (cào)", "cào comment " + post, false},
		{"D crawl verb (quét)", "quét comment " + post, false},
		{"D scrape verb", "scrape comments " + post, false},
		{"E bulk + url", "comment tất cả leads " + post, false},
		{"no url", "comment bài này", false},
		{"F lookalike host", "comment bài này https://facebook.com.evil.com/groups/x/permalink/1/", false},
		{"F lookalike host 2", "comment bài này https://evil-facebook.com/groups/x/permalink/1/", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := promptIsDirectPostComment(c.prompt); got != c.want {
				t.Errorf("promptIsDirectPostComment(%q) = %v, want %v", c.prompt, got, c.want)
			}
		})
	}
}

// D — crawl commands still route to scrape_comments, NOT comment_single_post.
func TestP0_CrawlCommandsStayScrapeComments(t *testing.T) {
	const post = "https://www.facebook.com/groups/123/posts/456/"
	for _, prompt := range []string{
		"cào comment " + post,
		"quét comment " + post,
		"scrape comments " + post,
	} {
		t.Run(prompt, func(t *testing.T) {
			action, _, ok := deterministicFacebookAction(prompt, 5, 0)
			if !ok || action != "scrape_comments" {
				t.Fatalf("action = %q (ok=%v), want scrape_comments", action, ok)
			}
		})
	}
}

// E — bulk commands still route to comment_all_leads, NOT comment_single_post
// (regression guard: the direct-comment bypass must not hijack bulk). These are
// the bulk-scope phrases the existing lexicon supports; "comment toàn bộ lead" is
// a PRE-EXISTING NLU gap ("toàn bộ" is not in lexCommentBulkScope, so on main it
// routes to search_groups) — orthogonal to this P0 routing fix, deferred to an
// NLU PR; not asserted here because P0 must not expand NLU coverage.
func TestP0_BulkCommandsStayAllLeads(t *testing.T) {
	for _, prompt := range []string{
		"comment tất cả leads",
		"comment tất cả lead",
		"comment cho các lead",
	} {
		t.Run(prompt, func(t *testing.T) {
			action, _, ok := deterministicFacebookAction(prompt, 5, 0)
			if !ok || action != "comment_all_leads" {
				t.Fatalf("action = %q (ok=%v), want comment_all_leads", action, ok)
			}
		})
	}
}

// F — lookalike / non-Facebook hosts must NOT be captured as a direct-comment post.
func TestP0_LookalikeHostsRejected(t *testing.T) {
	for _, prompt := range []string{
		"comment bài này https://facebook.com.evil.com/groups/ship.viet.my/permalink/4504452536547584/",
		"comment bài này https://evil-facebook.com/groups/ship.viet.my/permalink/4504452536547584/",
		"comment bài này https://example.com/groups/x/permalink/1/",
	} {
		t.Run(prompt, func(t *testing.T) {
			action, args, _ := deterministicFacebookAction(prompt, 5, 0)
			if action == "comment_single_post" && argStringFromMap(args, "post_url") != "" {
				t.Errorf("lookalike/non-FB must not be captured as post_url: %q -> %q", prompt, argStringFromMap(args, "post_url"))
			}
		})
	}
}

// Integration: the EXACT production prompt fires the early bypass BEFORE the brain
// and threads user_id + user_role into the deterministic args (so outbound can
// enforce execution-layer ownership). Brain is nil here, so reaching the fast-path
// without falling through to a generic response proves the bypass ran first.
func TestP0_DirectCommentBypassThreadsIdentity(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "p0-direct-comment.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	agent := NewAgent("test-key", "test-model", db)
	var gotAction string
	var gotArgs map[string]any
	agent.ActionHandler = func(action string, args map[string]any) (string, error) {
		gotAction = action
		gotArgs = args
		return "ok", nil
	}

	const orgID, userID int64 = 7, 99
	const role = "sales"
	const post = "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/"
	if _, err := agent.ProcessPromptForOrgWithUser(
		context.Background(), "Comment bài này cho tôi "+post, "dashboard", orgID, 0, userID, role); err != nil {
		t.Fatal(err)
	}
	if gotAction != "comment_single_post" {
		t.Fatalf("action = %q, want comment_single_post (direct-comment bypass before brain)", gotAction)
	}
	if got := argStringFromMap(gotArgs, "user_id"); got != "99" {
		t.Errorf("args[user_id] = %q, want \"99\"", got)
	}
	if got := argStringFromMap(gotArgs, "user_role"); got != role {
		t.Errorf("args[user_role] = %q, want %q", got, role)
	}
}
