package ai

import "testing"

// §7: a Facebook post URL + comment intent routes to comment_single_post (the
// direct-link flow), and "comment <bulk>" still routes to comment_all_leads.
func TestDeterministicFacebookAction_CommentSinglePost(t *testing.T) {
	const fbPost = "https://www.facebook.com/groups/123/posts/456/"
	cases := []struct {
		name    string
		prompt  string
		want    string
		wantURL string // expected args["post_url"], "" if none
	}{
		{"comment bài này + url", "comment bài này: " + fbPost, "comment_single_post", fbPost},
		{"comment lead này + url stays single post", "hãy comment lead này " + fbPost, "comment_single_post", fbPost},
		{"đăng comment vào post này + url", "đăng comment vào post này " + fbPost, "comment_single_post", fbPost},
		{"comment giúp tôi bài này + url", "comment giúp tôi bài này " + fbPost, "comment_single_post", fbPost},
		{"comment giúp tôi lead này + url", "comment giúp tôi lead này " + fbPost, "comment_single_post", fbPost},
		{"mobile facebook url", "comment bài này https://m.facebook.com/groups/1/posts/2", "comment_single_post", "https://m.facebook.com/groups/1/posts/2"},
		{"no url but bài này → single post (asks for link)", "soạn comment cho bài này", "comment_single_post", ""},
		{"no url but lead này → single post (asks for link)", "comment giúp tôi lead này", "comment_single_post", ""},

		{"bulk comment tất cả leads unchanged", "comment tất cả leads", "comment_all_leads", ""},
		{"bulk comment all leads unchanged", "comment tất cả lead", "comment_all_leads", ""},
		{"bulk comment các lead unchanged", "comment cho các lead", "comment_all_leads", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, args, ok := deterministicFacebookAction(c.prompt, 5, 0)
			if !ok || action != c.want {
				t.Fatalf("action = %q (ok=%v), want %q", action, ok, c.want)
			}
			if c.wantURL != "" {
				if got := argStringFromMap(args, "post_url"); got != c.wantURL {
					t.Errorf("post_url = %q, want %q", got, c.wantURL)
				}
			}
		})
	}
}

// A crawl verb means "scrape this post's comments", NOT post a comment — it must
// route to scrape_comments (unchanged), not the single-post comment intent.
func TestDeterministicFacebookAction_CrawlVerbStaysScrapeComments(t *testing.T) {
	action, _, ok := deterministicFacebookAction(
		"cào comment bài viết https://www.facebook.com/groups/123/posts/456/", 5, 0)
	if !ok || action != "scrape_comments" {
		t.Fatalf("crawl verb + comment + post url must route to scrape_comments, got %q (ok=%v)", action, ok)
	}
}

// Lookalike Facebook hosts and non-Facebook URLs must NOT be captured as a
// direct-comment post target (host anchoring via fburl).
func TestDeterministicFacebookAction_RejectsLookalikeAndNonFacebook(t *testing.T) {
	for _, prompt := range []string{
		"comment https://facebook.com.evil.com/posts/123",
		"comment https://notfacebook.com/posts/123",
		"comment https://example.com/posts/1",
	} {
		action, args, _ := deterministicFacebookAction(prompt, 5, 0)
		if action == "comment_single_post" && argStringFromMap(args, "post_url") != "" {
			t.Errorf("lookalike/non-FB must not be captured as post_url: prompt %q -> %q", prompt, argStringFromMap(args, "post_url"))
		}
	}
}
