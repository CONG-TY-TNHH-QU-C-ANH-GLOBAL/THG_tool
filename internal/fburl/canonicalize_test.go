package fburl

import (
	"reflect"
	"testing"
)

// Vanity group permalink grammar: /groups/{group_ref}/{permalink|posts}/{post_id}
// where group_ref is a DYNAMIC segment (numeric id OR vanity slug). The slug must
// be preserved in the canonical group permalink, not dropped to the global form.
func TestCanonicalizePostURL_VanityGroup(t *testing.T) {
	accept := map[string]string{
		// numeric group id — unchanged
		"https://www.facebook.com/groups/1312868109620530/permalink/2045056849734982/": "https://www.facebook.com/groups/1312868109620530/permalink/2045056849734982/",
		// vanity slug with dots — kept in the group permalink (was the production bug)
		"https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/": "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/",
		// vanity slug with dashes via /posts/ — normalized to /permalink/, group kept
		"https://www.facebook.com/groups/some-group-name/posts/1234567890/": "https://www.facebook.com/groups/some-group-name/permalink/1234567890/",
		// mobile host → www, slug kept
		"https://m.facebook.com/groups/ship.viet.my/permalink/4504452536547584/": "https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/",
	}
	for in, want := range accept {
		if got, ok := CanonicalizePostURL(in); !ok || got != want {
			t.Errorf("CanonicalizePostURL(%q) = (%q, %v), want (%q, true)", in, got, ok, want)
		}
	}
	reject := []string{
		"https://facebook.com.evil.com/groups/ship.viet.my/permalink/4504452536547584/", // lookalike host
		"https://evil-facebook.com/groups/ship.viet.my/permalink/4504452536547584/",      // lookalike host
		"https://notfacebook.com/groups/ship.viet.my/permalink/4504452536547584/",        // lookalike host
		"https://facebook.com/groups/ship.viet.my/",                                      // group shell, not a post
		"https://facebook.com/groups/ship.viet.my/members",                               // group members, not a post
	}
	for _, in := range reject {
		if _, ok := CanonicalizePostURL(in); ok {
			t.Errorf("must reject %q", in)
		}
	}
}

// Host anchoring: only genuine Facebook hosts are accepted; lookalikes that merely
// contain the brand text are rejected (security: substring matching let them in).
func TestFacebookHostAnchoring(t *testing.T) {
	reject := []string{
		"https://facebook.com.evil.com/posts/123",
		"https://notfacebook.com/posts/123",
		"https://fake-facebook.com/posts/1",
		"https://fb.com.evil.com/posts/1",
		"https://evil.com/?next=facebook.com/posts/123", // brand only in query
		"https://facebook.com@evil.com/posts/1",         // userinfo trick → host is evil.com
		"https://example.com/posts/1",
	}
	for _, in := range reject {
		if urls := ExtractFacebookURLs(in); len(urls) != 0 {
			t.Errorf("lookalike must be rejected, ExtractFacebookURLs(%q)=%v", in, urls)
		}
		if _, ok := CanonicalizePostURL(in); ok {
			t.Errorf("lookalike must not canonicalize: %q", in)
		}
		// IsFacebookURL (the brain's validation gate) must also reject the lookalike.
		if IsFacebookURL(in) {
			t.Errorf("IsFacebookURL must reject lookalike host: %q", in)
		}
	}
	for _, ok := range []string{
		"https://www.facebook.com/groups/1/posts/2", "https://m.facebook.com/x",
		"https://fb.com/y", "https://fb.watch/z",
	} {
		if !IsFacebookURL(ok) {
			t.Errorf("IsFacebookURL must accept genuine host: %q", ok)
		}
	}

	accept := map[string][]string{
		"xem https://www.facebook.com/groups/1/posts/2/ nhé": {"https://www.facebook.com/groups/1/posts/2/"},
		"https://m.facebook.com/groups/1/posts/2":            {"https://m.facebook.com/groups/1/posts/2"},
		"https://fb.watch/abc123/":                           {"https://fb.watch/abc123/"},
		"https://fb.com/somepage/posts/9":                    {"https://fb.com/somepage/posts/9"},
	}
	for in, want := range accept {
		if got := ExtractFacebookURLs(in); !reflect.DeepEqual(got, want) {
			t.Errorf("ExtractFacebookURLs(%q) = %v, want %v", in, got, want)
		}
	}
}

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
