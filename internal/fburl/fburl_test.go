package fburl

import "testing"

// CanonicalPostPermalink synthesises group post URLs using the /permalink/
// path. This is load-bearing: the /posts/ form post-2026 rejects
// top_level_post_id IDs (the "content isn't available" production bug),
// while /permalink/ accepts both story_fbid and top_level_post_id. Pin
// the form so a regression here re-introduces the dead-link bug.
func TestCanonicalPostPermalink_UsesPermalinkForm(t *testing.T) {
	t.Parallel()
	cases := []struct {
		group, post, want string
	}{
		{"123", "456", "https://www.facebook.com/groups/123/permalink/456/"},
		{"", "456", "https://www.facebook.com/permalink.php?story_fbid=456"},
		{"123", "", ""},
		{"", "", ""},
		// Whitespace tolerated.
		{" 123 ", " 456 ", "https://www.facebook.com/groups/123/permalink/456/"},
	}
	for _, c := range cases {
		if got := CanonicalPostPermalink(c.group, c.post); got != c.want {
			t.Errorf("CanonicalPostPermalink(%q, %q) = %q; want %q", c.group, c.post, got, c.want)
		}
	}
}

// ExtractFacebookPostID prefers /permalink/ over /posts/. The two paths
// can carry different IDs (story_fbid vs top_level_post_id) — when both
// patterns appear in a URL, the /permalink/ ID is the URL-resolvable one.
// Marker order in the implementation MUST keep /permalink/ ahead of /posts/.
func TestExtractFacebookPostID_PrefersPermalink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, url, want string
	}{
		{
			name: "URL has both /permalink/ and /posts/ in path — permalink wins",
			// Crafted URL where /posts/ appears first in the path string but
			// /permalink/ should still be preferred by the marker scan.
			url:  "https://www.facebook.com/groups/X/posts/999999/permalink/2019565862284081/",
			want: "2019565862284081",
		},
		{
			name: "plain /permalink/ URL",
			url:  "https://www.facebook.com/groups/123/permalink/456/",
			want: "456",
		},
		{
			name: "plain /posts/ URL — still extracted as last resort",
			url:  "https://www.facebook.com/groups/123/posts/789/",
			want: "789",
		},
		{
			name: "story_fbid query takes precedence over /posts/",
			url:  "https://www.facebook.com/x/posts/000?story_fbid=456",
			want: "456",
		},
		{
			name: "the failing production case — top_level_post_id in /posts/ still extracts",
			// Even though the URL is broken, ExtractFacebookPostID has to
			// return SOMETHING so the lead still gets recorded. The URL form
			// fix (CanonicalPostPermalink → /permalink/) is what makes the
			// resulting permalink resolve.
			url:  "https://www.facebook.com/groups/1312868109620530/posts/122216110760343408/",
			want: "122216110760343408",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractFacebookPostID(c.url); got != c.want {
				t.Errorf("ExtractFacebookPostID(%q) = %q; want %q", c.url, got, c.want)
			}
		})
	}
}

// LooksLikePostURL must accept the new /permalink/ canonical form
// since CanonicalPostPermalink now emits it. Without this, every
// freshly-synthesised URL would fail ValidateRouting.
func TestLooksLikePostURL_AcceptsPermalinkSynthesisOutput(t *testing.T) {
	t.Parallel()
	url := CanonicalPostPermalink("123", "456")
	if !LooksLikePostURL(url) {
		t.Fatalf("synthesised URL %q must pass LooksLikePostURL — otherwise repair → validate → drop loop", url)
	}
}
