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
		{
			name:    "primary URL is a bare group shell — no post id",
			in:      Input{SourceType: "post", PrimaryURL: "https://facebook.com/groups/123"},
			wantErr: true,
		},
		{
			name:    "primary URL is a profile shell — no post id",
			in:      Input{SourceType: "post", PrimaryURL: "https://facebook.com/some.user"},
			wantErr: true,
		},
		{
			name: "permalink.php with story_fbid is accepted",
			in:   Input{SourceType: "post", PrimaryURL: "https://www.facebook.com/permalink.php?story_fbid=12345"},
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

// TestCanonicalPostPermalink locks the synthesis used as the server-side
// rescue when the crawler emitted a group shell URL.
func TestCanonicalPostPermalink(t *testing.T) {
	cases := []struct {
		group, post, want string
	}{
		{"123", "456", "https://www.facebook.com/groups/123/posts/456/"},
		{"", "456", "https://www.facebook.com/permalink.php?story_fbid=456"},
		{"123", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := CanonicalPostPermalink(tc.group, tc.post)
		if got != tc.want {
			t.Errorf("CanonicalPostPermalink(%q, %q) = %q, want %q", tc.group, tc.post, got, tc.want)
		}
	}
}

// TestRepairPrimaryURL covers the IngestPost rescue path: when the
// crawler emits a group shell as PrimaryURL but supplies PostFBID + GroupFBID,
// the pipeline must rewrite the URL to a real post permalink BEFORE
// ValidateRouting runs.
func TestRepairPrimaryURL(t *testing.T) {
	t.Run("group shell + IDs rescue to canonical permalink", func(t *testing.T) {
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://www.facebook.com/groups/123",
			PostFBID:   "456",
			GroupFBID:  "123",
		}
		repairPrimaryURL(&in)
		want := "https://www.facebook.com/groups/123/posts/456/"
		if in.PrimaryURL != want {
			t.Errorf("PrimaryURL = %q, want %q", in.PrimaryURL, want)
		}
		if err := ValidateRouting(in); err != nil {
			t.Errorf("rescued lead must pass validator, got err=%v", err)
		}
	})
	t.Run("post URL untouched when already valid", func(t *testing.T) {
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://www.facebook.com/groups/123/posts/456",
			PostFBID:   "456",
			GroupFBID:  "123",
		}
		repairPrimaryURL(&in)
		if in.PrimaryURL != "https://www.facebook.com/groups/123/posts/456" {
			t.Errorf("valid post URL must not be rewritten, got %q", in.PrimaryURL)
		}
	})
	t.Run("group shell with no PostFBID stays broken", func(t *testing.T) {
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://www.facebook.com/groups/123",
		}
		repairPrimaryURL(&in)
		if err := ValidateRouting(in); err == nil {
			t.Error("expected validator to reject group shell with no PostFBID")
		}
	})
	t.Run("group shell with story_fbid in URL recovers post id", func(t *testing.T) {
		in := Input{
			SourceType: "post",
			PrimaryURL: "https://www.facebook.com/foo?story_fbid=789",
		}
		repairPrimaryURL(&in)
		// looksLikePostURL is already true — should be untouched, validator passes.
		if err := ValidateRouting(in); err != nil {
			t.Errorf("URL with story_fbid must validate, got err=%v", err)
		}
	})
}

func TestLooksLikePostURL(t *testing.T) {
	posts := []string{
		"https://facebook.com/groups/1/posts/2",
		"https://facebook.com/permalink/2",
		"https://facebook.com/x?story_fbid=99",
		"https://facebook.com/photo.php?fbid=42",
		"https://facebook.com/?multi_permalinks=1",
	}
	for _, u := range posts {
		if !LooksLikePostURL(u) {
			t.Errorf("LooksLikePostURL(%q) = false, want true", u)
		}
	}
	shells := []string{
		"",
		"https://facebook.com/groups/1",
		"https://facebook.com/some.user",
		"https://facebook.com/groups/1/?ref=feed",
	}
	for _, u := range shells {
		if LooksLikePostURL(u) {
			t.Errorf("LooksLikePostURL(%q) = true, want false", u)
		}
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
