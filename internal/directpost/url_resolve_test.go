package directpost

import (
	"strings"
	"testing"
)

// ResolveCommentURL is pure — assert every §7 URL-layer state.
func TestResolveCommentURL(t *testing.T) {
	const post = "https://www.facebook.com/groups/123/posts/456/"
	cases := []struct {
		name, prompt, postArg string
		wantBlocked           bool
		wantContains          string // substring of Message (blocked) or Canonical (ok)
	}{
		{"no url asks for link", "comment bài này", "", true, "gửi giúp tôi link"},
		{"two urls one only", "comment " + post + " và " + "https://www.facebook.com/groups/9/posts/9/", "", true, "chỉ gửi một link"},
		{"unsupported group shell", "comment bài này https://www.facebook.com/groups/123", "", true, "chưa được hỗ trợ"},
		{"valid normalizes to canonical", "comment bài này " + post, post, false, "https://www.facebook.com/groups/123/permalink/456/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ResolveCommentURL(c.prompt, c.postArg)
			if r.Blocked != c.wantBlocked {
				t.Fatalf("Blocked=%v want %v (msg=%q canonical=%q)", r.Blocked, c.wantBlocked, r.Message, r.Canonical)
			}
			hay := r.Message
			if !r.Blocked {
				hay = r.Canonical
			}
			if !strings.Contains(hay, c.wantContains) {
				t.Errorf("got %q, want contains %q", hay, c.wantContains)
			}
		})
	}
}
