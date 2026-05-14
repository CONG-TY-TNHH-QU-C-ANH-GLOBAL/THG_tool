package leadingest

import "testing"

// TestValidateRouting locks the lead routing contract: every lead must carry a
// usable POST url; a comment-sourced lead must resolve to its parent post.
func TestValidateRouting(t *testing.T) {
	cases := []struct {
		name    string
		in      Input
		wantErr bool
	}{
		{
			name: "post with a real post url",
			in:   Input{SourceType: "post", PrimaryURL: "https://facebook.com/groups/1/posts/2"},
		},
		{
			name: "empty source type defaults to post",
			in:   Input{PrimaryURL: "https://facebook.com/groups/1/posts/2"},
		},
		{
			name:    "post missing primary url",
			in:      Input{SourceType: "post", PrimaryURL: ""},
			wantErr: true,
		},
		{
			name:    "primary url is a comment-only link",
			in:      Input{SourceType: "post", PrimaryURL: "https://facebook.com/x?comment_id=99"},
			wantErr: true,
		},
		{
			name: "comment resolved to its parent post",
			in: Input{
				SourceType:   "comment",
				PrimaryURL:   "https://facebook.com/groups/1/posts/2",
				SecondaryURL: "https://facebook.com/groups/1/posts/2?comment_id=9",
			},
		},
		{
			name: "comment not resolved — primary equals comment link",
			in: Input{
				SourceType:   "comment",
				PrimaryURL:   "https://facebook.com/groups/1/posts/2",
				SecondaryURL: "https://facebook.com/groups/1/posts/2",
			},
			wantErr: true,
		},
		{
			name:    "comment with no parent at all",
			in:      Input{SourceType: "comment", PrimaryURL: ""},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRouting(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateRouting(%+v) err = %v, wantErr = %v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeSourceType(t *testing.T) {
	if got := normalizeSourceType(""); got != "post" {
		t.Errorf("empty -> %q, want post", got)
	}
	if got := normalizeSourceType("COMMENT"); got != "comment" {
		t.Errorf("COMMENT -> %q, want comment", got)
	}
	if got := normalizeSourceType("weird-value"); got != "post" {
		t.Errorf("unknown -> %q, want post (safe default)", got)
	}
}

// TestExtractFacebookPostID locks the URL-shape handling used as the cursor
// fallback when the crawler does not emit PostFBID explicitly.
func TestExtractFacebookPostID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://facebook.com/groups/123/posts/45678", "45678"},
		{"https://facebook.com/permalink/12345", "12345"},
		{"https://facebook.com/foo?story_fbid=98765&id=1", "98765"},
		{"https://facebook.com/groups/1/permalink/777/", "777"},
		{"https://facebook.com/groups/1/posts/2?comment_id=9", "2"},
		{"https://facebook.com/photo.php?fbid=42", "42"},
		{"https://facebook.com/x?a=1&fbid=99", "99"},
		{"https://facebook.com/random", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := ExtractFacebookPostID(tc.in); got != tc.want {
				t.Errorf("ExtractFacebookPostID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLooksLikeCommentOnlyURL(t *testing.T) {
	commentOnly := []string{
		"https://facebook.com/x?comment_id=99",
		"https://facebook.com/groups/1/comment/55",
	}
	for _, u := range commentOnly {
		if !looksLikeCommentOnlyURL(u) {
			t.Errorf("looksLikeCommentOnlyURL(%q) = false, want true", u)
		}
	}
	postContext := []string{
		"https://facebook.com/groups/1/posts/2",
		"https://facebook.com/groups/1/posts/2?comment_id=9",
		"https://facebook.com/permalink/2",
		"",
	}
	for _, u := range postContext {
		if looksLikeCommentOnlyURL(u) {
			t.Errorf("looksLikeCommentOnlyURL(%q) = true, want false", u)
		}
	}
}
