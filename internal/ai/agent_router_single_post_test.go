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
		{"no url but bài này → single post (asks for link)", "soạn comment cho bài này", "comment_single_post", ""},

		{"bulk comment all leads unchanged", "comment tất cả lead", "comment_all_leads", ""},
		{"bulk comment leads unchanged", "comment cho các lead", "comment_all_leads", ""},
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
// NOT be hijacked by the single-post comment intent.
func TestDeterministicFacebookAction_CrawlVerbIsNotSinglePost(t *testing.T) {
	action, _, ok := deterministicFacebookAction(
		"cào comment bài viết https://www.facebook.com/groups/123/posts/456/", 5, 0)
	if ok && action == "comment_single_post" {
		t.Fatalf("crawl verb must not route to comment_single_post, got %q", action)
	}
}
