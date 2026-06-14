package fburl

import "testing"

func TestCanonicalizePostURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"group posts form", "https://www.facebook.com/groups/123/posts/456/", "https://www.facebook.com/groups/123/permalink/456/", true},
		{"group permalink form", "https://www.facebook.com/groups/123/permalink/456/", "https://www.facebook.com/groups/123/permalink/456/", true},
		{"permalink.php story_fbid", "https://www.facebook.com/permalink.php?story_fbid=456&id=99", "https://www.facebook.com/permalink.php?story_fbid=456", true},
		{"group posts with tracking params", "https://www.facebook.com/groups/123/posts/456/?ref=share&mibextid=abc", "https://www.facebook.com/groups/123/permalink/456/", true},
		{"mobile host group post", "https://m.facebook.com/groups/123/posts/456/", "https://www.facebook.com/groups/123/permalink/456/", true},
		{"page posts numeric", "https://www.facebook.com/somepage/posts/456", "https://www.facebook.com/permalink.php?story_fbid=456", true},
		{"pfbid profile post strips query+host", "https://m.facebook.com/john/posts/pfbid02abcdefghij?ref=x", "https://www.facebook.com/john/posts/pfbid02abcdefghij", true},

		{"group shell (no post) unsupported", "https://www.facebook.com/groups/123", "", false},
		{"profile shell unsupported", "https://www.facebook.com/profile.php?id=123", "", false},
		{"comment-only link unsupported", "https://www.facebook.com/groups/123?comment_id=789", "", false},
		{"empty", "", "", false},
		{"non-facebook", "https://example.com/posts/1", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := CanonicalizePostURL(c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("CanonicalizePostURL(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
