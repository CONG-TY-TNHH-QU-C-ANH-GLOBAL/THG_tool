package facebook

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Moved verbatim from cmd/scraper/outbound_actions_test.go (Phase C/I) alongside the
// target-URL resolution it pins; assertions are unchanged.

// assertURLReason runs one (wantURL, wantReason) subtest. Shared by the resolver table
// tests so the identical assertion block isn't repeated per case (de-duplication).
func assertURLReason(t *testing.T, name, gotURL, gotReason, wantURL, wantReason string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		if gotURL != wantURL {
			t.Errorf("url = %q, want %q", gotURL, wantURL)
		}
		if gotReason != wantReason {
			t.Errorf("reason = %q, want %q", gotReason, wantReason)
		}
	})
}

func TestIsCommentableFacebookPostURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "profile post", url: "https://www.facebook.com/user/posts/123456789", want: true},
		{name: "group permalink", url: "https://www.facebook.com/groups/123/permalink/456/", want: true},
		{name: "group posts path", url: "https://www.facebook.com/groups/123/posts/456/", want: true},
		{name: "story fbid", url: "https://www.facebook.com/story.php?story_fbid=456&id=123", want: true},
		{name: "multi permalinks", url: "https://www.facebook.com/groups/123?multi_permalinks=456", want: true},
		{name: "photo", url: "https://www.facebook.com/photo.php?fbid=456", want: true},
		{name: "fb watch", url: "https://fb.watch/abc123/", want: true},
		{name: "group home is unsafe", url: "https://www.facebook.com/groups/123", want: false},
		{name: "profile home is unsafe", url: "https://www.facebook.com/profile.php?id=123", want: false},
		{name: "facebook home is unsafe", url: "https://www.facebook.com/", want: false},
		{name: "external url", url: "https://example.com/posts/123", want: false},
		{name: "empty", url: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCommentableFacebookPostURL(tt.url); got != tt.want {
				t.Fatalf("IsCommentableFacebookPostURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveOutboundTargetURL(t *testing.T) {
	const groupPost = "https://www.facebook.com/groups/123/posts/456/"

	// Commentable SourceURL passes through unchanged for comment across accepted source
	// types. Looped (not 5 identical struct rows) so the homogeneous shape is not a
	// duplicated block; behavior/coverage is identical.
	passThrough := []struct{ name, sourceType, url string }{
		{"post source comment msg uses SourceURL", "post", groupPost},
		{"comment source comment msg uses SourceURL (parent post)", "comment", groupPost},
		{"prompt_target source comment msg uses SourceURL", "prompt_target", "https://www.facebook.com/user/posts/789"},
		{"empty source type defaults to SourceURL path", "", groupPost},
		{"uppercased source type normalises", "POST", groupPost},
	}
	for _, tt := range passThrough {
		gotURL, gotReason := ResolveOutboundTargetURL(models.Lead{SourceType: tt.sourceType, SourceURL: tt.url}, "comment")
		assertURLReason(t, tt.name, gotURL, gotReason, tt.url, "")
	}

	// Heterogeneous cases (skips, FBID reconstruction, inbox, non-comment), one row per line.
	tests := []struct {
		name       string
		lead       models.Lead
		msgType    string
		wantURL    string
		wantReason string
	}{
		{name: "inbox source type rejected for comment", lead: models.Lead{SourceType: "inbox", SourceURL: groupPost}, msgType: "comment", wantReason: "unrouted_source_type"},
		{name: "unknown source type rejected", lead: models.Lead{SourceType: "weird_new_type", SourceURL: groupPost}, msgType: "comment", wantReason: "unrouted_source_type"},
		{name: "photo SourceURL with group+post FBID reconstructs canonical", lead: models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111&set=gm.222", GroupFBID: "123", PostFBID: "456"}, msgType: "comment", wantURL: groupPost},
		{name: "photo SourceURL without group_fbid still skipped", lead: models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111", PostFBID: "456"}, msgType: "comment", wantReason: "missing_post_permalink"},
		{name: "photo SourceURL without post_fbid still skipped", lead: models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111", GroupFBID: "123"}, msgType: "comment", wantReason: "missing_post_permalink"},
		{name: "non-commentable URL no FBIDs skipped", lead: models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123"}, msgType: "comment", wantReason: "missing_post_permalink"},
		{name: "inbox msg uses AuthorURL ignoring SourceURL", lead: models.Lead{SourceType: "post", SourceURL: groupPost, AuthorURL: "https://www.facebook.com/user.42"}, msgType: "inbox", wantURL: "https://www.facebook.com/user.42"},
		{name: "inbox msg missing AuthorURL", lead: models.Lead{SourceType: "inbox"}, msgType: "inbox", wantReason: "missing_target"},
		{name: "non-comment msg with empty SourceURL", lead: models.Lead{SourceType: "post", SourceURL: ""}, msgType: "group_post", wantReason: "missing_target"},
		{name: "non-comment msg passes through without commentable check", lead: models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123"}, msgType: "group_post", wantURL: "https://www.facebook.com/groups/123"},
	}
	for _, tt := range tests {
		gotURL, gotReason := ResolveOutboundTargetURL(tt.lead, tt.msgType)
		assertURLReason(t, tt.name, gotURL, gotReason, tt.wantURL, tt.wantReason)
	}
}

func TestCanonicalGroupPostURLFromFBIDs(t *testing.T) {
	tests := []struct {
		name      string
		groupFBID string
		postFBID  string
		want      string
	}{
		{name: "both present", groupFBID: "123", postFBID: "456", want: "https://www.facebook.com/groups/123/posts/456/"},
		{name: "missing group", groupFBID: "", postFBID: "456", want: ""},
		{name: "missing post", groupFBID: "123", postFBID: "", want: ""},
		{name: "both missing", groupFBID: "", postFBID: "", want: ""},
		{name: "whitespace trimmed", groupFBID: "  123  ", postFBID: "  456  ", want: "https://www.facebook.com/groups/123/posts/456/"},
		{name: "whitespace-only treated as empty", groupFBID: "   ", postFBID: "456", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalGroupPostURLFromFBIDs(tt.groupFBID, tt.postFBID); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
