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

// ExtractFacebookEntityID must recognise BOTH the numeric and the
// compact pfbid identifier forms. It is the identity helper the
// verifier defense-in-depth check relies on — when this returns "",
// the verifier downgrades a "sent" outcome to context_drift. Every
// shape Facebook actually serves must round-trip; every shape we
// cannot trust must return "" so the caller fails closed.
func TestExtractFacebookEntityID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, url, want string
	}{
		{
			name: "numeric /posts/ in group permalink",
			url:  "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/",
			want: "2019673682273299",
		},
		{
			name: "pfbid /posts/ in profile permalink",
			url:  "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
			want: "pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
		},
		{
			name: "pfbid followed by ?comment_id query — body terminates at ?",
			url:  "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl?comment_id=1293405342441584",
			want: "pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
		},
		{
			name: "/permalink/ form",
			url:  "https://www.facebook.com/groups/X/permalink/456/",
			want: "456",
		},
		{
			name: "?story_fbid= query",
			url:  "https://www.facebook.com/story.php?story_fbid=999&id=1",
			want: "999",
		},
		{
			name: "/photo.php?fbid= form",
			url:  "https://www.facebook.com/photo.php?fbid=123",
			want: "123",
		},
		{
			name: "/watch/?v= form",
			url:  "https://www.facebook.com/watch/?v=987654321",
			want: "987654321",
		},
		{
			name: "empty URL fails closed",
			url:  "",
			want: "",
		},
		{
			name: "group/page profile shell — no post id",
			url:  "https://www.facebook.com/groups/123",
			want: "",
		},
		{
			name: "garbage string",
			url:  "not a url",
			want: "",
		},
		{
			name: "accidental pfbid substring inside path (no URL boundary) is rejected",
			// This URL has "pfbid" embedded but not preceded by /=?&, so it's
			// not a real identifier — must NOT match.
			url:  "https://www.facebook.com/about/contentpfbidsomestuff",
			want: "",
		},
		{
			name: "pfbid too short to be a real id",
			// pfbid token bodies are long base64-ish blobs. Refuse short matches.
			url:  "https://www.facebook.com/x/posts/pfbid123",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractFacebookEntityID(c.url); got != c.want {
				t.Errorf("ExtractFacebookEntityID(%q) = %q; want %q", c.url, got, c.want)
			}
		})
	}
}

// SameFacebookEntity is the comparison primitive the verifier uses.
// "Same" must be tight enough that two URLs only return true when both
// independently resolve to the same id. The "fail closed" requirement
// (empty id ⇒ never equal, even to another empty id) is load-bearing:
// a caller that gets an empty page_url_after from the extension
// MUST be treated as unverified, not as "trivially matching" the
// (also unparseable) noise.
func TestSameFacebookEntity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "numeric == numeric — same group post",
			a:    "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/",
			b:    "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/?comment_id=999",
			want: true,
		},
		{
			name: "/permalink/ form normalises to /posts/ numeric",
			a:    "https://www.facebook.com/groups/X/posts/2019673682273299/",
			b:    "https://www.facebook.com/groups/X/permalink/2019673682273299/",
			want: true,
		},
		{
			name: "story_fbid query normalises to /posts/ numeric",
			a:    "https://www.facebook.com/groups/X/posts/2019673682273299/",
			b:    "https://www.facebook.com/story.php?story_fbid=2019673682273299&id=1",
			want: true,
		},
		{
			name: "pfbid == pfbid — same profile post even with comment anchor",
			a:    "https://www.facebook.com/u/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl",
			b:    "https://www.facebook.com/u/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl?comment_id=1293405342441584",
			want: true,
		},
		{
			name: "INCIDENT: numeric group post vs different pfbid profile post — must NOT match",
			a:    "https://www.facebook.com/groups/1312868109620530/posts/2019673682273299/",
			b:    "https://www.facebook.com/luong.the.hung.800599/posts/pfbid02R3qUXYGfCsyT4HbSWdFWgaccCeXg7qFCiPqxDueupCEpPghznjaVNDBCxYVPT9VZl?comment_id=1293405342441584",
			want: false,
		},
		{
			name: "different numeric ids in same group — must NOT match",
			a:    "https://www.facebook.com/groups/X/posts/111/",
			b:    "https://www.facebook.com/groups/X/posts/222/",
			want: false,
		},
		{
			name: "empty actual fails closed",
			a:    "https://www.facebook.com/groups/X/posts/2019673682273299/",
			b:    "",
			want: false,
		},
		{
			name: "empty target fails closed",
			a:    "",
			b:    "https://www.facebook.com/groups/X/posts/2019673682273299/",
			want: false,
		},
		{
			name: "both empty fails closed — empty does NOT equal empty",
			a:    "",
			b:    "",
			want: false,
		},
		{
			name: "garbage on either side fails closed",
			a:    "https://www.facebook.com/groups/X/posts/2019673682273299/",
			b:    "not a url",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameFacebookEntity(c.a, c.b); got != c.want {
				t.Errorf("SameFacebookEntity(%q, %q) = %v; want %v", c.a, c.b, got, c.want)
			}
		})
	}
}
